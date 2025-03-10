package perf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/automaxprocs/maxprocs"
	"gopkg.in/yaml.v2"
)

const (
	minMaxResult = 8192
	contentType  = "application/x-yaml"
)

// RunConf 运行配置
type RunConf struct {
	RunTime      int                 `yaml:"RunTime" json:"RunTime"`
	Debug        bool                `yaml:"Debug" json:"Debug"`
	RemoteServer map[string][]string `yaml:"RemoteServer" json:"RemoteServer"`
	ParamsConfs  []*ParamsConf       `yaml:"Params" json:"Params"`
	TcpGroups    []*TcpGroup         `yaml:"TcpGroups" json:"TcpGroups"`
	HTTPconfs    []*HTTPconf         `yaml:"HTTPConfs" json:"HTTPConfs"`
	ctx          *RunCtx
	Report       *Report
	running      int32 // 添加运行状态标志
}

// RunCtx 运行上下文
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
	return ReadRunConfByByte(buf)
}

func ReadRunConfByByte(body []byte) (*RunConf, error) {
	var rc RunConf

	// Try JSON first
	err := json.Unmarshal(body, &rc)
	if err == nil {
		return &rc, nil
	}

	// Fall back to YAML if JSON fails
	err = yaml.Unmarshal(body, &rc)
	if err != nil {
		return nil, err
	}
	return &rc, nil
}

var sendOnCloseError interface{}

func (rc *RunConf) init() error {
	// 初始化上下文
	ctx := &RunCtx{
		wg:    &sync.WaitGroup{},
		debug: rc.Debug,
	}
	ctx.ctx, ctx.cancel = context.WithCancel(context.Background())
	rc.ctx = ctx

	// 初始化请求映射
	reqMap := make(map[string]*HTTPconf, len(rc.HTTPconfs))
	paramsMap := GetParamsMap(rc.ParamsConfs)

	for _, httpConf := range rc.HTTPconfs {
		httpConf.paramsMap = paramsMap
		reqMap[httpConf.Name] = httpConf
	}

	// 计算最大结果数
	maxResult := rc.calculateMaxResult()
	report := NewReport(ctx, maxResult)
	rc.Report = report

	// 初始化TCP组
	for _, tg := range rc.TcpGroups {
		tg.Init(ctx, report, reqMap)
	}

	return nil
}

func (rc *RunConf) calculateMaxResult() int {
	maxResult := 0
	for _, tg := range rc.TcpGroups {
		maxResult += tg.MaxQPS
	}
	if maxResult < minMaxResult {
		maxResult = minMaxResult
	}
	return maxResult
}

// Stop 停止测试运行
func (rc *RunConf) Stop() {
	if atomic.CompareAndSwapInt32(&rc.running, 1, 0) {
		rc.shutdown()
	}
}

func (rc *RunConf) Run() {
	// 设置运行状态
	if !atomic.CompareAndSwapInt32(&rc.running, 0, 1) {
		fmt.Println("Test is already running")
		return
	}

	if len(rc.RemoteServer) != 0 {
		rc.RemoteRun()
		atomic.StoreInt32(&rc.running, 0)
		return
	}

	// 初始化
	_, _ = maxprocs.Set()
	if err := rc.init(); err != nil {
		fmt.Printf("初始化失败: %v\n", err)
		atomic.StoreInt32(&rc.running, 0)
		return
	}

	// 启动连接池
	rc.ctx.wg.Add(1)
	go PoolPrint(rc.ctx.wg)

	for _, tg := range rc.TcpGroups {
		rc.ctx.wg.Add(1)
		go tg.InitPool()
	}
	rc.ctx.wg.Wait()

	// 启动测试
	rc.ctx.wg.Add(2)
	go rc.Report.Printer()
	go rc.timer()

	for _, tg := range rc.TcpGroups {
		rc.ctx.wg.Add(1)
		go tg.Run()
	}
	rc.ctx.wg.Wait()

	atomic.StoreInt32(&rc.running, 0)
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
	rc.Report = &Report{
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
		go rc.Report.RemotePrinter(remoteIp)
	}
	rc.ctx.wg.Wait()
	rc.Report.RemotePrintResult()
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
	req.Header.Set("Content-Type", contentType)
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
	defer signal.Stop(signals)
	defer close(signals)

	for {
		select {
		case <-signals:
			rc.shutdown()
			return
		default:
			if !rc.isRunning() || rc.shouldStop() {
				rc.shutdown()
				return
			}
		}
	}
}

func (rc *RunConf) shouldStop() bool {
	if rc.Report.StartTime.IsZero() {
		return false
	}
	return time.Since(rc.Report.StartTime).Seconds() > float64(rc.RunTime)
}

// isRunning 检查测试是否正在运行
func (rc *RunConf) isRunning() bool {
	return atomic.LoadInt32(&rc.running) == 1
}

func (rc *RunConf) shutdown() {
	// 先调用 cancel 通知所有 goroutine 停止
	rc.ctx.cancel()

	// 使用 WaitGroup 等待所有连接处理完成
	var wg sync.WaitGroup
	for _, tg := range rc.TcpGroups {
		if tg.pool != nil && !tg.pool.IsClosed() {
			wg.Add(1)
			go func(pool *ConnPool) {
				defer wg.Done()
				pool.Close()
			}(tg.pool)
		}
	}
	wg.Wait()
}
