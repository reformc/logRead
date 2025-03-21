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
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
)

const lines = 1000 //查看实时日志时从多少行开始查看

var addr = flag.String("addr", ":9198", "http service address")
var htmlPath = flag.String("htmlPath", "", "the html file path")

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
	stop   chan struct{}
	finish chan struct{}
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
}

func newSendLog(c *websocket.Conn) *sendLog {
	return &sendLog{
		logCell: nil,
		c:       c,
		mt:      0,
	}
}

// 检查是否存在日志发送线程通道，如果存在则发送停止信号并等待停止
// 然后创建新的线程通道并启动线程。
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

// 读取websocket信息并解析为请求结构体
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
		if string(message) == "0" {
			continue
		}
		err = json.Unmarshal(message, req)
		if err != nil {
			//log.Println(string(message))
			//log.Println(err)
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

// 启动一个docker日志发送线程
func (s *sendLog) dockerLog(containerName string, flag *logThread) {
	defer close(flag.finish)
	dockerC, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("error when create dockerClient ", err)
	}
	defer dockerC.Close()
	reader, err := dockerC.ContainerLogs(context.TODO(), containerName, types.ContainerLogsOptions{
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
			if len(b) < 9 {
				continue
			}
			if err != nil {
				return
			}
			if len(b) < 9 {
				log.Println(string(b))
			} else if s.c.WriteMessage(s.mt, b[8:]) != nil {
				return
			}
		}

	}
}

func (s *sendLog) dockerHistoryLog(containerName, since, until string, grep []byte, lines int, flag *logThread) {
	if lines > 10000 {
		lines = 10000
	}
	if lines == 0 {
		lines = 10000
	}
	defer close(flag.finish)
	dockerC, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("error when create dockerClient ", err)
	}
	defer dockerC.Close()
	reader, err := dockerC.ContainerLogs(context.TODO(), containerName, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
		Since:      timeUtc(since), //浏览器自定义时间框会有T
		Until:      timeUtc(until),
		//Tail:       strconv.Itoa(lines),
	})
	log.Println(containerName, timeUtc(since), timeUtc(until))
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
					if len(b) < 9 {
						//log.Println(string(b))
					} else if s.c.WriteMessage(s.mt, b[8:]) != nil {
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
	if lines > 10000 {
		lines = 10000
	}
	if lines == 0 {
		lines = 10000
	}
	defer close(flag.finish)
	ctx, cancle := context.WithCancel(context.Background())
	defer cancle()
	command := "journalctl"
	if since != "" {
		command = command + " --since=\"" + since + "\"" //若使用浏览器自带时间选择框会有T,需要将T换成空格
	}
	if until != "" {
		command = command + " --until=\"" + until + "\""
	}
	command = command + " -u " + serviceName
	if string(grep) != "" {
		command = command + " |grep " + string(grep)
	}
	cmd, err := getOutput(ctx, "sh", "-c", command) //linux系统将cmd改成sh
	log.Println(command)
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
			if s.c.WriteMessage(s.mt, msg) != nil {
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

// 显示服务列表
func serviceList(w http.ResponseWriter, r *http.Request) {
	var err error

	dockerC, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("error when create dockerClient ", err)
	}
	defer dockerC.Close()
	containers, err := dockerC.ContainerList(context.Background(), types.ContainerListOptions{})
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

	//	w.Write([]byte(`
	//docker:<select id="select_docker" onchange="log_docker();">
	//   <option value =""> -- </option>
	//	`))
	dockerServiceList := new(ServiceListObj)
	dockerServiceList.ServiceType = "docker"
	for _, upCell := range up {
		//w.Write([]byte(fmt.Sprintf(`<option value ="%s">%s</option>`, upCell[1], upCell[1])))
		dockerServiceList.List = append(dockerServiceList.List, SeviceCell{Name: upCell[1], Value: upCell[1]})
	}
	for _, downCell := range down {
		//w.Write([]byte(fmt.Sprintf(`<option value ="%s">*%s</option>`, downCell[1], downCell[1])))
		dockerServiceList.List = append(dockerServiceList.List, SeviceCell{Name: downCell[1], Value: downCell[1]})
	}

	//w.Write([]byte("\n</select><br>"))

	cmd := exec.Command("sh", "-c", "systemctl list-units -all|grep -E 'reform_|hzbit_|ymd_'")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout // 标准输出
	cmd.Stderr = &stderr // 标准错误
	err = cmd.Run()
	if err != nil {
		w.Write([]byte(err.Error()))
	}
	_, errStr := stdout.String(), stderr.String()
	if errStr != "" {
		w.Write([]byte(errStr))
	} else {
		var up [][]string
		var down [][]string
		for {
			if str, err := stdout.ReadString([]byte("\n")[0]); err != nil {
				break
			} else {
				par := split(strings.ReplaceAll(strings.ReplaceAll(str, "●", ""), "*", " "))
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
		systemctlList := new(ServiceListObj)
		systemctlList.ServiceType = "journalctl"
		//		w.Write([]byte(`
		//system:<select id="select_systemd" onchange="log_systemd();">
		//    <option value =""> -- </option>
		//	`))
		for _, upCell := range up {
			//w.Write([]byte(fmt.Sprintf(`<option value ="%s">%s</option>`, upCell[1], upCell[1])))
			systemctlList.List = append(systemctlList.List, SeviceCell{Name: upCell[1], Value: upCell[1]})
		}
		for _, downCell := range down {
			//w.Write([]byte(fmt.Sprintf(`<option value ="%s">*%s</option>`, downCell[1], downCell[1])))
			systemctlList.List = append(systemctlList.List, SeviceCell{Name: downCell[1], Value: downCell[1]})
		}
		b, err := json.MarshalIndent([]ServiceListObj{*dockerServiceList, *systemctlList}, " ", " ")
		if err != nil {
			w.Write([]byte(err.Error()))
		} else {
			w.Write(b)
		}
	}
}

func indexServe(w http.ResponseWriter, r *http.Request) {
	content, err := os.ReadFile(*htmlPath + "/index.html")
	if err != nil {
		w.Write([]byte(err.Error()))
	} else {
		w.Write(content)
	}
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if *htmlPath == "" {
		*htmlPath, _ = os.Getwd()
	}
	//test()
	log.SetFlags(0)
	http.HandleFunc("/readlog/list", serviceList)
	http.HandleFunc("/readlog/main", indexServe)
	//http.HandleFunc("/readlog/index.html", indexServe)
	http.Handle("/readlog/", http.FileServer(http.Dir(*htmlPath+"/")))
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

func timeUtc(str string) string {
	stamp, err := time.ParseInLocation("2006-01-02 15:04:05", str, time.Local)
	if err != nil {
		return strings.ReplaceAll(str, " ", "T") + "Z"
	}
	return stamp.UTC().Format("2006-01-02T15:04:05Z")
}
