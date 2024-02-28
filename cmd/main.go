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

type strListFlag []string

func (f *strListFlag) String() string {
	return fmt.Sprintf("%v", []string(*f))
}

func (f *strListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

var (
	confName   = flag.String("conf", "", "指定运行的yaml文件,其他选项会失效,优先执行yaml文件")
	isServer   = flag.Bool("S", false, "以服务端的方式运行,-P指定监听端口")
	serverPort = flag.String("P", "8888", "-S下生效,指定监听端口")
	urlStr     = flag.String("u", "", "URL地址:http://test.com:8080/test")
	reqThread  = flag.Int("c", 100, "请求线程数,默认100")
	runTime    = flag.Int("t", 10, "运行时间,默认10s")
	rps        = flag.Int("R", 0, "限制RPS发送请求速率,默认不限制")
	postBody   = flag.String("data", "", "发送body:-data 'a=1&b=2'")
	method     = flag.String("m", "GET", "请求方法")
	maxReqest  = flag.Int("k", 100, "设置每个TCP最多发送多少请求,默认100")
	debug      = flag.Bool("debug", false, "是否打开打印调试信息")
	headerList strListFlag
)

func init() {
	flag.Var(&headerList, "H", "自定义头:-H 'Content-Type: application/json'")
	flag.Parse()
}

func main() {
	if *isServer {
		server.Start(*serverPort)
	} else if *confName != "" {
		RunConf, err := perf.ReadRunConfByFile(*confName)
		if err != nil {
			log.Fatal("Read RunConf By File fail", err.Error())
		}
		RunConf.Run()
	} else {
		runCmdLine()
	}
}

func runCmdLine() {
	dst, err := perf.GetDstByUrl(*urlStr)
	if err != nil {
		log.Fatal("url is error:", err.Error())
	}
	var headerMap map[string]string = map[string]string{}
	for _, headerStr := range headerList {
		kv := strings.SplitN(headerStr, ":", 2)
		if len(kv) != 2 {
			continue
		}
		headerMap[kv[0]] = kv[1]
	}
	newhttpconf := &perf.HTTPconf{
		Name:   "test",
		Proto:  "HTTP1/1",
		Method: *method,
		URI:    *urlStr,
		Body:   *postBody,
		Header: headerMap,
	}

	newTcpGroup := &perf.TcpGroup{
		Name:            "test",
		MaxTcpConnPerIP: *reqThread,
		TcpConnThread:   1000,
		TcpCreatThread:  10,
		TcpCreatRate:    10000,
		SrcIP:           []string{},
		MaxQPS:          *rps,
		Dst:             dst,
		ReqThread:       *reqThread,
		MaxReqest:       100,
		IsHttps:         perf.UrlIsHttps(*urlStr),
		SendHttp:        []string{"test"},
	}
	newRunConf := &perf.RunConf{
		RunTime:   *runTime,
		Debug:     *debug,
		TcpGroups: []*perf.TcpGroup{newTcpGroup},
		HTTPconfs: []*perf.HTTPconf{newhttpconf},
	}
	newRunConf.Run()
}
