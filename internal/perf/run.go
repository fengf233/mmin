package perf

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/automaxprocs/maxprocs"
	"gopkg.in/yaml.v2"
)

type RunConf struct {
	RunTime      int                 `yaml:"RunTime"`
	Debug        bool                `yaml:"Debug"`
	RemoteServer map[string][]string `yaml:"RemoteServer"`
	ParamsConfs  []*ParamsConf       `yaml:"Params"`
	TcpGroups    []*TcpGroup         `yaml:"TcpGroups"`
	HTTPconfs    []*HTTPconf         `yaml:"HTTPConfs"`
	ctx          *RunCtx
	report       *Report
}

type RunCtx struct {
	wg     *sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
	debug  bool
}

func ReadRunConfByFile(filename string) (*RunConf, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var rc RunConf
	err = yaml.Unmarshal(buf, &rc)
	if err != nil {
		return nil, err
	}
	return &rc, nil
}

func ReadRunConfByByte(body []byte) (*RunConf, error) {
	var rc RunConf
	err := yaml.Unmarshal(body, &rc)
	if err != nil {
		return nil, err
	}
	return &rc, nil
}

var sendOnCloseError interface{}

func (rc *RunConf) init() {
	ctx := &RunCtx{
		debug: rc.Debug,
	}
	var wg sync.WaitGroup
	ctx.wg = &wg
	ctx.ctx, ctx.cancel = context.WithCancel(context.Background())
	var reqMap map[string]*HTTPconf = map[string]*HTTPconf{}
	paramsMap := GetParamsMap(rc.ParamsConfs)
	for _, httpConf := range rc.HTTPconfs {
		httpConf.paramsMap = paramsMap
		reqMap[httpConf.Name] = httpConf
	}
	maxResult := 0
	for _, tg := range rc.TcpGroups {
		maxResult += tg.MaxQPS
	}
	if maxResult < 8192 {
		maxResult = 8192
	}
	report := NewReport(ctx, maxResult)
	for _, tg := range rc.TcpGroups {
		tg.Init(ctx, report, reqMap)
	}
	rc.report = report
	rc.ctx = ctx

	//捕获管道关闭异常,用于多个生产者当管道被消费者关闭时panic问题
	defer func() {
		sendOnCloseError = recover()
	}()
	func() {
		cc := make(chan struct{}, 1)
		close(cc)
		cc <- struct{}{}
	}()
}

func (rc *RunConf) Run() *Report {
	if len(rc.RemoteServer) != 0 {
		rc.RemoteRun()
		return nil
	}
	//初始化参数,tcp池
	_, _ = maxprocs.Set()
	rc.init()
	rc.ctx.wg.Add(1)
	go PoolPrint(rc.ctx.wg)
	for _, tg := range rc.TcpGroups {
		rc.ctx.wg.Add(1)
		go tg.InitPool()
	}
	rc.ctx.wg.Wait()
	//开始运行发送请求
	rc.ctx.wg.Add(1)
	go rc.report.Printer()
	rc.ctx.wg.Add(1)
	go rc.timer()
	for _, tg := range rc.TcpGroups {
		rc.ctx.wg.Add(1)
		go tg.Run()
	}
	rc.ctx.wg.Wait()
	return rc.report
}

func (rc *RunConf) RemoteRun() {
	ctx := &RunCtx{
		debug: rc.Debug,
	}
	var wg sync.WaitGroup
	ctx.wg = &wg
	ctx.ctx, ctx.cancel = context.WithCancel(context.Background())
	rc.ctx = ctx
	var rwlock sync.RWMutex
	rc.report = &Report{
		Success:    0,
		Receive:    0,
		Send:       0,
		AvgRate:    0,
		AllReqTime: 0,
		Respcode:   map[int]int{},
		ErrMap:     map[string]int{},
		ctx:        ctx,
		rwlock:     &rwlock,
	}
	for remoteIp, confList := range rc.RemoteServer {
		rc.ctx.wg.Add(1)
		go rc.sendRemoteConf(remoteIp, confList)
		rc.ctx.wg.Add(1)
		go rc.report.RemotePrinter(remoteIp)
	}
	rc.ctx.wg.Wait()
	rc.report.RemotePrintResult()
}

func (rc *RunConf) sendRemoteConf(remoteDst string, confList []string) {
	defer rc.ctx.wg.Done()
	newRunConf := &RunConf{
		RunTime:   rc.RunTime,
		Debug:     rc.Debug,
		HTTPconfs: rc.HTTPconfs,
	}
	var newTcpGroups []*TcpGroup
	for _, groupName := range confList {
		for _, tcpGroup := range rc.TcpGroups {
			if groupName == tcpGroup.Name {
				newTcpGroups = append(newTcpGroups, tcpGroup)
			}
		}
	}
	newRunConf.TcpGroups = newTcpGroups
	yamlData, err := yaml.Marshal(newRunConf)
	if err != nil {
		fmt.Println("remoteDst:", remoteDst, "err:", err.Error())
		return
	}
	body := bytes.NewReader(yamlData)
	req, err := http.NewRequest("POST", "http://"+remoteDst+"/run", body)
	if err != nil {
		fmt.Println("remoteDst:", remoteDst, "err:", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("remoteDst:", remoteDst, "err:", err.Error())
		return
	}
	respbody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("remoteDst:", remoteDst, "err:", err.Error())
		return
	}
	if string(respbody) != "start" {
		fmt.Println("remoteDst:", remoteDst, "err:", string(respbody))
		return
	}
	defer resp.Body.Close()
}

func (rc *RunConf) timer() {
	defer rc.ctx.wg.Done()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-signals:
			rc.ctx.cancel()
			for _, tg := range rc.TcpGroups {
				if tg.pool == nil {
					os.Exit(0)
				}
				tg.pool.Close()
			}
			signal.Stop(signals)
			close(signals)
			// os.Exit(0)
			return
		default:
			if rc.report.StartTime.IsZero() {
				continue
			}
			if time.Now().Sub(rc.report.StartTime).Seconds() > float64(rc.RunTime) {
				// fmt.Println("sgo:", runtime.NumGoroutine())
				rc.ctx.cancel()
				for _, tg := range rc.TcpGroups {
					tg.pool.Close()
				}
				// //调试用
				// time.Sleep(2 * time.Second)
				// fmt.Println("figo:", runtime.NumGoroutine())
				// // 访问/debug/pprof/goroutine?debug=2
				// go func() {
				// 	fmt.Println("pprof start...")
				// 	fmt.Println(http.ListenAndServe(":9876", nil))
				// }()
				signal.Stop(signals)
				close(signals)
				return
			}
		}
	}
}
