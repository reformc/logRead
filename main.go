package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gorilla/websocket"
)

const (
	lines           = 1000 // 查看实时日志时从多少行开始查看
	maxHistoryLines = 10000
	maxLogLineBytes = 4 << 20
)

var addr = flag.String("addr", ":9198", "http service address")
var htmlPath = flag.String("htmlPath", "", "the html file path")
var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@:-]{0,254}$`)

type serviceReqwest struct {
	LogType     string `json:"log_type"`
	ServiceType string `json:"service_type"`
	ServiceName string `json:"service_name"`
	Since       string `json:"since"`
	Until       string `json:"until"`
	Grep        string `json:"grep"`
	Lines       int    `json:"lines"`
}

type logThread struct {
	ctx    context.Context
	cancel context.CancelFunc
	finish chan struct{}
	mu     sync.Mutex
	reader io.ReadCloser
}

type ServiceListObj struct {
	ServiceType string       `json:"serviceType"`
	List        []SeviceCell `json:"list"`
}

type SeviceCell struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func newLogThread() *logThread {
	ctx, cancel := context.WithCancel(context.Background())
	return &logThread{
		ctx:    ctx,
		cancel: cancel,
		finish: make(chan struct{}),
	}
}

func (l *logThread) close() {
	l.cancel()
	l.mu.Lock()
	reader := l.reader
	l.reader = nil
	l.mu.Unlock()
	if reader != nil {
		_ = reader.Close()
	}
}

func (l *logThread) setReader(reader io.ReadCloser) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	select {
	case <-l.ctx.Done():
		_ = reader.Close()
		return false
	default:
		l.reader = reader
		return true
	}
}

func getOutput(ctx context.Context, name string, args ...string) (<-chan []byte, error) {
	res := make(chan []byte, 100)
	cmd := exec.CommandContext(ctx, name, args...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return res, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdoutPipe.Close()
		return res, err
	}
	go func() {
		defer close(res)
		defer func() {
			if err := cmd.Wait(); err != nil && ctx.Err() == nil {
				log.Printf("%s failed: %v: %s", name, err, strings.TrimSpace(stderr.String()))
			}
		}()
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 64*1024), maxLogLineBytes)
		for scanner.Scan() {
			msg := append(bytes.Clone(scanner.Bytes()), '\n')
			select {
			case res <- msg:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			log.Printf("read %s output: %v", name, err)
		}
	}()
	return res, nil
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     sameOrigin,
}

type sendLog struct {
	logCell *logThread
	c       *websocket.Conn
	mt      int
	writeMu sync.Mutex
}

func newSendLog(c *websocket.Conn) *sendLog {
	return &sendLog{
		logCell: nil,
		c:       c,
		mt:      websocket.BinaryMessage,
	}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func validServiceName(name string) bool {
	return serviceNamePattern.MatchString(name)
}

func validRequest(req *serviceReqwest) bool {
	if req.ServiceType != "docker" && req.ServiceType != "systemd" {
		return false
	}
	if req.LogType != "realtime" && req.LogType != "history" {
		return false
	}
	return validServiceName(req.ServiceName)
}

func (s *sendLog) writeMessage(messageType int, data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.c.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return s.c.WriteMessage(messageType, data)
}

func (s *sendLog) sendEnd() {
	_ = s.writeMessage(s.mt, []byte("-------->message send over<--------"))
}

// 检查是否存在日志发送线程通道，如果存在则发送停止信号并等待停止
// 然后创建新的线程通道并启动线程。
func (s *sendLog) work(req *serviceReqwest, cell *logThread) {
	switch req.ServiceType {
	case "docker":
		switch req.LogType {
		case "realtime":
			go s.dockerLog(req.ServiceName, cell)
		case "history":
			go s.dockerHistoryLog(req.ServiceName, req.Since, req.Until, []byte(req.Grep), req.Lines, cell)
		}
	case "systemd":
		switch req.LogType {
		case "realtime":
			go s.systemLog(req.ServiceName, cell)
		case "history":
			go s.systemHistoryLog(req.ServiceName, req.Since, req.Until, []byte(req.Grep), req.Lines, cell)
		}
	}
}

// 读取websocket信息并解析为请求结构体
func (s *sendLog) read() {
	defer func() {
		if s.logCell != nil {
			s.logCell.close()
			<-s.logCell.finish
		}
	}()
	for {
		mt, message, err := s.c.ReadMessage()
		if err != nil {
			return
		}
		_ = s.c.SetReadDeadline(time.Now().Add(90 * time.Second))
		req := new(serviceReqwest)
		if string(message) == "0" || string(message) == `"0"` {
			continue
		}
		err = json.Unmarshal(message, req)
		if err != nil {
			log.Printf("invalid websocket request: %v", err)
			continue
		}
		if !validRequest(req) {
			log.Printf("rejected websocket request: invalid service type, log type, or service name")
			continue
		}
		if s.logCell != nil {
			s.logCell.close()
			<-s.logCell.finish
		}
		s.mt = mt
		s.logCell = newLogThread()
		s.work(req, s.logCell)
	}
}

// 启动一个docker日志发送线程
func (s *sendLog) dockerLog(containerName string, cell *logThread) {
	defer close(cell.finish)
	dockerC, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("create docker client: %v", err)
		return
	}
	defer func() { _ = dockerC.Close() }()
	inspect, err := dockerC.ContainerInspect(cell.ctx, containerName)
	if err != nil {
		log.Printf("inspect container %q: %v", containerName, err)
		return
	}
	reader, err := dockerC.ContainerLogs(cell.ctx, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       strconv.Itoa(lines),
	})
	if err != nil {
		log.Printf("read container %q logs: %v", containerName, err)
		return
	}
	if !cell.setReader(reader) {
		return
	}
	output := dockerLogOutput(reader, inspect.Config != nil && inspect.Config.Tty)
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64*1024), maxLogLineBytes)
	for scanner.Scan() {
		message := append(bytes.Clone(scanner.Bytes()), '\n')
		select {
		case <-cell.ctx.Done():
			return
		default:
			if s.writeMessage(s.mt, message) != nil {
				return
			}
		}
	}
	if err := scanner.Err(); err != nil && cell.ctx.Err() == nil {
		log.Printf("scan container %q logs: %v", containerName, err)
	}
}

func dockerLogOutput(reader io.ReadCloser, tty bool) io.Reader {
	if tty {
		return reader
	}
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pipeWriter, pipeWriter, reader)
		_ = pipeWriter.CloseWithError(err)
	}()
	return pipeReader
}

func normalizedLineLimit(lines int) int {
	if lines <= 0 || lines > maxHistoryLines {
		return maxHistoryLines
	}
	return lines
}

func (s *sendLog) dockerHistoryLog(containerName, since, until string, grep []byte, lines int, cell *logThread) {
	lines = normalizedLineLimit(lines)
	defer close(cell.finish)
	dockerC, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("create docker client: %v", err)
		s.sendEnd()
		return
	}
	defer func() { _ = dockerC.Close() }()
	inspect, err := dockerC.ContainerInspect(cell.ctx, containerName)
	if err != nil {
		log.Printf("inspect container %q: %v", containerName, err)
		s.sendEnd()
		return
	}
	reader, err := dockerC.ContainerLogs(cell.ctx, containerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
		Since:      timeUtc(since),
		Until:      timeUtc(until),
	})
	if err != nil {
		log.Printf("read container %q history: %v", containerName, err)
		s.sendEnd()
		return
	}
	if !cell.setReader(reader) {
		return
	}
	output := dockerLogOutput(reader, inspect.Config != nil && inspect.Config.Tty)
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64*1024), maxLogLineBytes)
	for scanner.Scan() {
		message := append(bytes.Clone(scanner.Bytes()), '\n')
		select {
		case <-cell.ctx.Done():
			return
		default:
			if len(grep) == 0 || bytes.Contains(message, grep) {
				if lines > 0 {
					lines--
					if s.writeMessage(s.mt, message) != nil {
						return
					}
				} else {
					s.sendEnd()
					return
				}
			}
		}
	}
	if err := scanner.Err(); err != nil && cell.ctx.Err() == nil {
		log.Printf("scan container %q history: %v", containerName, err)
	}
	if cell.ctx.Err() == nil {
		s.sendEnd()
	}
}

func (s *sendLog) systemHistoryLog(serviceName, since, until string, grep []byte, lines int, cell *logThread) {
	lines = normalizedLineLimit(lines)
	defer close(cell.finish)
	args := []string{"--no-pager"}
	if since != "" {
		args = append(args, "--since", since)
	}
	if until != "" {
		args = append(args, "--until", until)
	}
	args = append(args, "-u", serviceName)
	cmd, err := getOutput(cell.ctx, "journalctl", args...)
	if err != nil {
		log.Printf("start journalctl history for %q: %v", serviceName, err)
		s.sendEnd()
		return
	}
	for {
		select {
		case msg, ok := <-cmd:
			if !ok {
				s.sendEnd()
				return
			}
			if len(grep) > 0 && !bytes.Contains(msg, grep) {
				continue
			}
			if lines == 0 {
				s.sendEnd()
				return
			}
			lines--
			if err = s.writeMessage(s.mt, msg); err != nil {
				return
			}
		case <-cell.ctx.Done():
			return
		}
	}
}

func (s *sendLog) systemLog(serviceName string, cell *logThread) {
	defer close(cell.finish)
	cmd, err := getOutput(
		cell.ctx,
		"journalctl",
		"--follow",
		"-n", strconv.Itoa(lines),
		"-u", serviceName,
	)
	if err != nil {
		log.Printf("start journalctl for %q: %v", serviceName, err)
		return
	}

	for {
		select {
		case msg, ok := <-cmd:
			if !ok {
				return
			}
			if s.writeMessage(s.mt, msg) != nil {
				return
			}
		case <-cell.ctx.Done():
			return
		}
	}
}

func wsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(64 << 10)
	_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.SetPongHandler(func(string) error {
		return c.SetReadDeadline(time.Now().Add(90 * time.Second))
	})
	logT := newSendLog(c)
	logT.read()
}

// 显示服务列表
func serviceList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	dockerC, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		http.Error(w, "docker is unavailable", http.StatusServiceUnavailable)
		log.Printf("create docker client: %v", err)
		return
	}
	defer func() { _ = dockerC.Close() }()
	containers, err := dockerC.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		http.Error(w, "docker is unavailable", http.StatusServiceUnavailable)
		log.Printf("list containers: %v", err)
		return
	}
	dockerServiceList := ServiceListObj{ServiceType: "docker"}
	for _, item := range containers {
		if len(item.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(item.Names[0], "/")
		cell := SeviceCell{Name: name, Value: name}
		if item.State == "running" {
			dockerServiceList.List = append(dockerServiceList.List, cell)
		}
	}
	for _, item := range containers {
		if len(item.Names) == 0 || item.State == "running" {
			continue
		}
		name := strings.TrimPrefix(item.Names[0], "/")
		dockerServiceList.List = append(dockerServiceList.List, SeviceCell{Name: name, Value: name})
	}

	output, err := exec.CommandContext(
		ctx,
		"systemctl",
		"list-units",
		"--all",
		"--type=service",
		"--no-legend",
		"--plain",
	).Output()
	if err != nil {
		http.Error(w, "systemd is unavailable", http.StatusServiceUnavailable)
		log.Printf("list systemd services: %v", err)
		return
	}
	var running []SeviceCell
	var stopped []SeviceCell
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || !wantedSystemdUnit(fields[0]) {
			continue
		}
		name := strings.TrimSuffix(fields[0], ".service")
		cell := SeviceCell{Name: name, Value: name}
		if fields[3] == "running" {
			running = append(running, cell)
		} else {
			stopped = append(stopped, cell)
		}
	}
	if err := scanner.Err(); err != nil {
		http.Error(w, "failed to parse systemd services", http.StatusInternalServerError)
		log.Printf("parse systemd services: %v", err)
		return
	}
	systemctlList := ServiceListObj{
		ServiceType: "journalctl",
		List:        append(running, stopped...),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode([]ServiceListObj{dockerServiceList, systemctlList}); err != nil {
		log.Printf("write service list: %v", err)
	}
}

func wantedSystemdUnit(name string) bool {
	return strings.HasPrefix(name, "reform_") ||
		strings.HasPrefix(name, "hzbit_") ||
		strings.HasPrefix(name, "ymd_")
}

func indexServe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, filepath.Join(*htmlPath, "index.html"))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; "+
				"base-uri 'none'; frame-ancestors 'none'",
		)
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if *htmlPath == "" {
		*htmlPath, _ = os.Getwd()
	}
	log.SetFlags(0)
	mux := http.NewServeMux()
	mux.HandleFunc("/readlog/list", serviceList)
	mux.HandleFunc("/readlog/main", indexServe)
	mux.Handle("/readlog/", http.FileServer(http.Dir(*htmlPath+"/")))
	mux.HandleFunc("/readlog/wsapi", wsAPI)
	server := &http.Server{
		Addr:              *addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	fmt.Println(*addr)
	log.Fatal(server.ListenAndServe())
}

func timeUtc(str string) string {
	if str == "" {
		return ""
	}
	stamp, err := time.ParseInLocation("2006-01-02 15:04:05", str, time.Local)
	if err != nil {
		return str
	}
	return stamp.UTC().Format("2006-01-02T15:04:05Z")
}
