package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
)

const lines = 50 //查看实时日志时从多少行开始查看

var addr = flag.String("addr", ":9198", "http service address")
var htmlFile = flag.String("htmlPath", "/code/golang/readLog/log.html", "the html file path")
var dockerClient *client.Client

type serviceReqwest struct {
	LogType     string `json:"log_type"`
	ServiceType string `json:"service_type"`
	ServiceName string `json:"service_name"`
	Since       string `json:"since"`
	Until       string `json:"until"`
	Grep        string `json:"grep"`
	Lines       int    `json:"lines"`
}

type historyReqwest struct {
	ServiceType string `json:"service_type"`
	ServiceName string `json:"service_name"`
	Since       string `json:"since"`
	Until       string `json:"until"`
	Grep        string `json:"grep"`
}

type logThread struct {
	stop   chan struct{}
	finish chan struct{}
	reader io.ReadCloser
}

func newLogThread() *logThread {
	return &logThread{
		stop:   make(chan struct{}, 1),
		finish: make(chan struct{}, 1),
	}
}

func (l *logThread) close() {
	if l.reader != nil {
		l.reader.Close()
	}
	close(l.stop)
}

func getOutput(ctx context.Context, name string, args ...string) (chan []byte, error) {
	res := make(chan []byte, 100)
	cmd := exec.CommandContext(ctx, name, args...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdoutPipe.Close()
		//close(res)
		return res, err
	}
	go func() {
		defer close(res)
		//defer stdoutPipe.Close()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			res <- scanner.Bytes()
		}
	}()
	go func() {
		defer stdoutPipe.Close()
		if err = cmd.Run(); err != nil {
			return
		}
	}()
	return res, nil
}

var upgrader = websocket.Upgrader{
	// 解决跨域问题
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type sendLog struct {
	logCell *logThread
	c       *websocket.Conn
	mt      int
	state   bool
}

func newSendLog(c *websocket.Conn) *sendLog {
	return &sendLog{
		logCell: nil,
		c:       c,
		mt:      0,
	}
}

//检查是否存在日志发送线程通道，如果存在则发送停止信号并等待停止
//然后创建新的线程通道并启动线程。
func (s *sendLog) work(req *serviceReqwest) {
	switch req.ServiceType {
	case "docker":
		switch req.LogType {
		case "realtime":
			go s.dockerLog(req.ServiceName, s.logCell)
		case "history":
			go s.dockerHistoryLog(req.ServiceName, req.Since, req.Until, []byte(req.Grep), req.Lines, s.logCell)
		default:
		}
	case "systemd":
		switch req.LogType {
		case "realtime":
			go s.systemLog(req.ServiceName, s.logCell)
		case "history":
			go s.systemHistoryLog(req.ServiceName, req.Since, req.Until, []byte(req.Grep), req.Lines, s.logCell)
		default:
		}
	}
}

//读取websocket信息并解析为请求结构体
func (s *sendLog) read() {
	defer func(aa *sendLog) {
		if aa.logCell != nil {
			aa.logCell.close()
		}
	}(s)
	for {
		mt, message, err := s.c.ReadMessage()
		if err != nil {
			return
		}
		req := new(serviceReqwest)
		err = json.Unmarshal(message, req)
		if err != nil {
			log.Println(err)
			continue
		}
		if s.logCell == nil {
			s.mt = mt
			s.logCell = newLogThread()
		} else {
			s.logCell.close()
			<-s.logCell.finish
			s.logCell = newLogThread()
		}
		s.work(req)
	}
}

//启动一个docker日志发送线程
func (s *sendLog) dockerLog(containerName string, flag *logThread) {
	defer close(flag.finish)
	reader, err := dockerClient.ContainerLogs(context.TODO(), containerName, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       strconv.Itoa(lines),
	})
	if err != nil {
		log.Fatal("error when containerLogs", err)
	}
	flag.reader = reader
	r := bufio.NewReader(reader)
	for {
		select {
		case <-flag.stop:
			return
		default:
			b, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			if s.c.WriteMessage(s.mt, b[8:]) != nil {
				return
			}
		}
	}
}

func (s *sendLog) dockerHistoryLog(containerName, since, until string, grep []byte, lines int, flag *logThread) {
	if lines > 1000 {
		lines = 1000
	}
	defer close(flag.finish)
	reader, err := dockerClient.ContainerLogs(context.TODO(), containerName, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
		Since:      since,
		Until:      until,
		//Tail:       strconv.Itoa(lines),
	})
	//fmt.Println(since, until, lines) //------------------------------------------------------------------------
	if err != nil {
		log.Fatal("error when containerLogs", err)
	}
	flag.reader = reader
	r := bufio.NewReader(reader)
	for {
		select {
		case <-flag.stop:
			return
		default:
			b, err := r.ReadBytes('\n')
			if err != nil {
				_ = s.c.WriteMessage(s.mt, []byte("-------->message send over<--------"))
				return
			}
			if bytes.Contains(b, grep) {
				if lines > 0 {
					lines--
					if s.c.WriteMessage(s.mt, b[8:]) != nil {
						return
					}
				} else {
					_ = s.c.WriteMessage(s.mt, []byte("-------->message send over<--------"))
					return
				}
			}
		}
	}
}

func (s *sendLog) systemHistoryLog(serviceName, since, until string, grep []byte, lines int, flag *logThread) {
	if lines > 1000 {
		lines = 1000
	}
	defer close(flag.finish)
	ctx, cancle := context.WithCancel(context.Background())
	defer cancle()
	cmd, err := getOutput(ctx, "sh", "-c", fmt.Sprintf("journalctl -n %d --since=\"%s\" --unitl=\"%s\" -u %s|grep %s", lines, since, until, serviceName, string(grep))) //linux系统将cmd改成sh
	//log.Println("docker", "logs", "-f", "--tail=10", req.ServiceName)
	if err != nil {
		log.Println(err)
		return
	}
	for {
		select {
		case msg, ok := <-cmd:
			if !ok {
				_ = s.c.WriteMessage(s.mt, []byte("-------->message send over<--------"))
				return
			}
			if lines > 0 {
				lines--
				if err = s.c.WriteMessage(s.mt, msg); err != nil {
					log.Println(err, "终止")
					return
				}
			} else {
				_ = s.c.WriteMessage(s.mt, []byte("-------->message send over<--------"))
				return
			}
		case <-flag.stop:
			return
		}
	}
}

func (s *sendLog) systemLog(serviceName string, flag *logThread) {
	defer close(flag.finish)
	ctx, cancle := context.WithCancel(context.Background())
	defer cancle()
	cmd, err := getOutput(ctx, "sh", "-c", fmt.Sprintf("journalctl -f -n %d -u %s", lines, serviceName)) //linux系统将cmd改成sh
	//log.Println("docker", "logs", "-f", "--tail=10", req.ServiceName)
	if err != nil {
		log.Println(err)
		return
	}
	for {
		select {
		case msg := <-cmd:
			if err = s.c.WriteMessage(s.mt, msg); err != nil {
				log.Println(err, "终止")
				return
			}
		case <-flag.stop:
			return
		}
	}
}

func wsAPI(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	logT := newSendLog(c)
	logT.read()
}

//显示服务列表
func serviceList(w http.ResponseWriter, r *http.Request) {
	var err error

	dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("error when create dockerClient ", err)
	}
	defer dockerClient.Close()
	containers, err := dockerClient.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	var up [][]string
	var down [][]string
	for _, container := range containers {
		if container.State == "running" {
			up = append(up, []string{"docker", strings.ReplaceAll(container.Names[0], "/", "")})
		} else {
			down = append(down, []string{"docker", strings.ReplaceAll(container.Names[0], "/", "")})
		}
	}

	w.Write([]byte(`
docker:<select id="select_docker" onchange="log_docker();">
    <option value =""> -- </option>
	`))
	for _, upCell := range up {
		w.Write([]byte(fmt.Sprintf(`<option value ="%s">%s</option>`, upCell[1], upCell[1])))
	}
	for _, downCell := range down {
		w.Write([]byte(fmt.Sprintf(`<option value ="%s">*%s</option>`, downCell[1], downCell[1])))
	}

	w.Write([]byte("\n</select><br>"))

	cmd := exec.Command("sh", "-c", "systemctl list-units -all|grep -E 'reform|hzbit'")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout // 标准输出
	cmd.Stderr = &stderr // 标准错误
	err = cmd.Run()
	if err != nil {
		w.Write([]byte(err.Error()))
	}
	_, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	if errStr != "" {
		w.Write([]byte(errStr))
	} else {
		var up [][]string
		var down [][]string
		for {
			if str, err := stdout.ReadString([]byte("\n")[0]); err != nil {
				break
			} else {
				par := split(strings.ReplaceAll(str, "●", ""))
				if len(par) != 5 {
					w.Write([]byte("err,systemd name have ' '?"))
					return
				}
				if par[3] == "running" {
					up = append(up, []string{"systemd", strings.ReplaceAll(par[0], ".service", "")})
					//w.Write([]byte(fmt.Sprintf(`<a herf="javascript:void(0)" onclick="log_connect(this);" type="%s"><font color=green>%s</font></a><br>
					//`, "docker", par[0])))
				} else {
					down = append(down, []string{"systemd", strings.ReplaceAll(par[0], ".service", "")})
					//w.Write([]byte(fmt.Sprintf(`%s,`, par[0])))
				}
				//fmt.Println(strings.Split(str, ","))
			}
		}
		w.Write([]byte(`
system:<select id="select_systemd" onchange="log_systemd();">
    <option value =""> -- </option>
	`))
		for _, upCell := range up {
			w.Write([]byte(fmt.Sprintf(`<option value ="%s">%s</option>`, upCell[1], upCell[1])))
		}
		for _, downCell := range down {
			w.Write([]byte(fmt.Sprintf(`<option value ="%s">*%s</option>`, downCell[1], downCell[1])))
		}
		w.Write([]byte("\n</select><br>"))
		w.Write([]byte("\nsince<input id='input_since' type=\"datetime-local\" size=\"15\" name=\"input3\" />"))
		w.Write([]byte("\nuntil<input id='input_until' type=\"datetime-local\" size=\"15\" name=\"input4\" /><br>"))
		w.Write([]byte("\nlines<input id='input_lines' type=\"number\" size=\"15\" name=\"input5\" />"))
		w.Write([]byte("\ngrep<input id='input_grep' type=\"text\" size=\"15\" name=\"input6\" />"))
		w.Write([]byte("\n<button type=\"button\" onclick=history()>查询</button><br>"))
	}
}

func fileServe(w http.ResponseWriter, r *http.Request) {
	content, err := ioutil.ReadFile(*htmlFile)
	if err != nil {
		w.Write([]byte(err.Error()))
	} else {
		w.Write(content)
	}
}

func main() {
	flag.Parse()
	//test()
	log.SetFlags(0)
	http.HandleFunc("/readlog/list", serviceList)
	http.HandleFunc("/readlog", fileServe)
	http.HandleFunc("/readlog/wsapi", wsAPI)
	fmt.Println(*addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func split(str string) []string {
	var res []string
	str = strings.ReplaceAll(str, "\"", "")
	str = strings.ReplaceAll(str, "\n", "")
	str = strings.ReplaceAll(str, "\r", "")
	for _, c := range strings.Split(str, " ") {
		if c != "" && c != " " {
			res = append(res, c)
		}
	}
	return res
}
