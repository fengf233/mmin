/*
 * This file was created by fengf233.
 * Email: fengf233 <fengfeng2333@126.com>
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"mmin/internal/perf"
	"mmin/internal/server"
	"strings"
)

const (
	defaultPort      = "8888"
	defaultThreads   = 100
	defaultRunTime   = 10
	defaultMaxReq    = 100
	defaultMethod    = "GET"
	defaultTcpCreate = 10
	defaultTcpRate   = 10000
)

// strListFlag 用于处理多个header参数
type strListFlag []string

func (f *strListFlag) String() string {
	return fmt.Sprintf("%v", []string(*f))
}

func (f *strListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

// Config 命令行配置
type Config struct {
	confName   string
	isRemote   bool
	isWeb      bool
	serverPort string
	urlStr     string
	reqThread  int
	runTime    int
	rps        int
	postBody   string
	method     string
	maxRequest int
	debug      bool
	headers    strListFlag
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.confName, "conf", "", "配置文件路径 (yaml,json格式)")
	// flag.BoolVar(&cfg.isRemote, "remote", false, "作为远程节点")
	flag.BoolVar(&cfg.isWeb, "web", false, "启动web服务")
	flag.StringVar(&cfg.serverPort, "p", defaultPort, "服务器监听端口")
	flag.StringVar(&cfg.urlStr, "u", "", "目标URL (例如: http://example.com:8080/path)")
	flag.IntVar(&cfg.reqThread, "c", defaultThreads, "并发线程数")
	flag.IntVar(&cfg.runTime, "t", defaultRunTime, "运行时间(秒)")
	flag.IntVar(&cfg.rps, "r", 0, "每秒请求数限制(0表示不限制)")
	flag.StringVar(&cfg.postBody, "d", "", "POST请求体数据")
	flag.StringVar(&cfg.method, "X", defaultMethod, "HTTP请求方法")
	flag.IntVar(&cfg.maxRequest, "k", defaultMaxReq, "单个TCP连接最大请求数")
	flag.BoolVar(&cfg.debug, "v", false, "显示详细调试信息")
	flag.Var(&cfg.headers, "H", "自定义HTTP头 (可重复使用)")

	flag.Parse()
	return cfg
}

func main() {
	cfg := parseFlags()

	if cfg.isRemote {
		if err := server.StartRemoteServer(cfg.serverPort); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
		return
	}

	if cfg.isWeb {
		server.StartWebServer(cfg.serverPort)
		return
	}

	if cfg.confName != "" {
		runWithConfig(cfg.confName)
		return
	}

	runWithCommandLine(cfg)
}

func runWithConfig(confName string) {
	runConf, err := perf.ReadRunConfByFile(confName)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	runConf.Run()
}

func runWithCommandLine(cfg *Config) {
	dst, err := perf.GetDstByUrl(cfg.urlStr)
	if err != nil {
		log.Fatalf("Invalid URL: %v", err)
	}

	httpConf := createHTTPConfig(cfg)
	tcpGroup := createTCPGroup(cfg, dst)

	runConf := &perf.RunConf{
		RunTime:   cfg.runTime,
		Debug:     cfg.debug,
		TcpGroups: []*perf.TcpGroup{tcpGroup},
		HTTPconfs: []*perf.HTTPconf{httpConf},
	}
	runConf.Run()
}

func createHTTPConfig(cfg *Config) *perf.HTTPconf {
	headerMap := parseHeaders(cfg.headers)
	return &perf.HTTPconf{
		Name:   "test",
		Proto:  "HTTP1/1",
		Method: cfg.method,
		URI:    cfg.urlStr,
		Body:   cfg.postBody,
		Header: headerMap,
	}
}

func createTCPGroup(cfg *Config, dst string) *perf.TcpGroup {
	return &perf.TcpGroup{
		Name:            "test",
		MaxTcpConnPerIP: cfg.reqThread,
		TcpConnThread:   defaultThreads,
		TcpCreatThread:  defaultTcpCreate,
		TcpCreatRate:    defaultTcpRate,
		SrcIP:           []string{},
		MaxQPS:          cfg.rps,
		Dst:             dst,
		ReqThread:       cfg.reqThread,
		MaxReqest:       cfg.maxRequest,
		IsHttps:         perf.UrlIsHttps(cfg.urlStr),
		SendHttp:        []string{"test"},
	}
}

func parseHeaders(headers []string) map[string]string {
	headerMap := make(map[string]string)
	for _, header := range headers {
		if kv := strings.SplitN(header, ":", 2); len(kv) == 2 {
			headerMap[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return headerMap
}
