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
	isServer   bool
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

	flag.StringVar(&cfg.confName, "conf", "", "指定运行的yaml文件,其他选项会失效,优先执行yaml文件")
	flag.BoolVar(&cfg.isServer, "S", false, "以服务端的方式运行,-P指定监听端口")
	flag.StringVar(&cfg.serverPort, "P", defaultPort, "-S下生效,指定监听端口")
	flag.StringVar(&cfg.urlStr, "u", "", "URL地址:http://test.com:8080/test")
	flag.IntVar(&cfg.reqThread, "c", defaultThreads, "请求线程数,默认100")
	flag.IntVar(&cfg.runTime, "t", defaultRunTime, "运行时间,默认10s")
	flag.IntVar(&cfg.rps, "R", 0, "限制RPS发送请求速率,默认不限制")
	flag.StringVar(&cfg.postBody, "data", "", "发送body:-data 'a=1&b=2'")
	flag.StringVar(&cfg.method, "m", defaultMethod, "请求方法")
	flag.IntVar(&cfg.maxRequest, "k", defaultMaxReq, "设置每个TCP最多发送多少请求,默认100")
	flag.BoolVar(&cfg.debug, "debug", false, "是否打开打印调试信息")
	flag.Var(&cfg.headers, "H", "自定义头:-H 'Content-Type: application/json'")

	flag.Parse()
	return cfg
}

func main() {
	cfg := parseFlags()

	if cfg.isServer {
		if err := server.Start(cfg.serverPort); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
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
