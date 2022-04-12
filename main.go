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
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", ":9198", "http service address")
var htmlFile = flag.String("htmlPath", "/code/golang/readLog/log.html", "the html file path")
var dockerClient *client.Client

//const htmlFile = "G:/log.html"
//const htmlFile = "/code/golang/readLog/log.html"

type serviceReqwest struct {
	ServiceType string `json:"service_type"`
	ServiceName string `json:"service_name"`
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
		close(res)
		return res, err
	}
	go func() {
		defer stdoutPipe.Close()
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			/*
				data, err := simplifiedchinese.GB18030.NewDecoder().Bytes(scanner.Bytes())//windows系统需要转码
				if err != nil {
					//log.Println(scanner.Bytes())
				}
				res <- data
			*/
			res <- scanner.Bytes()
		}
	}()
	go func() {
		defer close(res)
		//defer log.Println(name,"运行结束")
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

func sendlog(closeCh chan struct{}, c *websocket.Conn, req serviceReqwest, mt int) {
	switch req.ServiceType {
	case "docker":

	}
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
		go s.dockerLog(req.ServiceName, s.logCell)
	case "systemd":
		go s.systemLog(req.ServiceName, s.logCell)
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
		Tail:       "10",
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
			//fmt.Println(string(b))
			if s.c.WriteMessage(s.mt, b[8:]) != nil {
				return
			}
		}
	}
}

func (s *sendLog) systemLog(serviceName string, flag *logThread) {
	defer close(flag.finish)
	ctx, cancle := context.WithCancel(context.Background())
	defer cancle()
	cmd, err := getOutput(ctx, "sh", "-c", "journalctl -f -u "+serviceName) //linux系统将cmd改成sh
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

func serviceList(w http.ResponseWriter, r *http.Request) {
	//cmd := exec.Command("cmd", "/C", "docker ps -a --format \"{{.Names}},{{.Status}}\"")
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

	for _, upCell := range up {
		w.Write([]byte(fmt.Sprintf(`<a herf="javascript:void(0)" onclick="log_connect(this);" type="%s"><font color=green>%s</font></a>, `, "docker", upCell[1])))
	}
	w.Write([]byte("<br>"))
	for _, downCell := range down {
		w.Write([]byte(fmt.Sprintf(`%s,`, downCell[1])))
	}

	w.Write([]byte("<br>"))

	cmd := exec.Command("sh", "-c", "systemctl list-units -all|grep reform")
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
		for _, upCell := range up {
			w.Write([]byte(fmt.Sprintf(`<a herf="javascript:void(0)" onclick="log_connect(this);" type="%s"><font color=green>%s</font></a>, `, upCell[0], upCell[1])))
		}
		w.Write([]byte("<br>"))
		for _, downCell := range down {
			w.Write([]byte(fmt.Sprintf(`<a herf="javascript:void(0)" onclick="log_connect(this);" type="%s"><font color=black>%s</font></a>, `, downCell[0], downCell[1])))
		}
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
