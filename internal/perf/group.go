package perf

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type TcpGroup struct {
	Name            string   `yaml:"Name"`
	MaxTcpConnPerIP int      `yaml:"MaxTcpConnPerIP"`
	TcpConnThread   int      `yaml:"TcpConnThread"`
	TcpCreatThread  int      `yaml:"TcpCreatThread"`
	TcpCreatRate    int      `yaml:"TcpCreatRate"`
	SrcIP           []string `yaml:"SrcIP"`
	MaxQPS          int      `yaml:"MaxQps"`
	Dst             string   `yaml:"Dst"`
	ReqThread       int      `yaml:"ReqThread"`
	MaxReqest       int      `yaml:"MaxReqest"`
	IsHttps         bool     `yaml:"IsHttps"`
	SendHttp        []string `yaml:"SendHttp"`

	sendHttpConfs []*HTTPconf
	pool          *ConnPool
	rl            *rate.Limiter
	r             *Report
	ctx           *RunCtx
}

func (tg *TcpGroup) Init(ctx *RunCtx, r *Report, reqMap map[string]*HTTPconf) {
	for _, sendHttpName := range tg.SendHttp {
		httpConf := reqMap[sendHttpName]
		if httpConf != nil {
			err := httpConf.SetReqBytes()
			if err != nil {
				log.Fatalln("TcpGroup httpConf init fail", err.Error())
			}
			tg.sendHttpConfs = append(tg.sendHttpConfs, httpConf)
		}
	}
	tg.rl = rate.NewLimiter(rate.Limit(tg.MaxQPS), 1)
	tg.ctx = ctx
	tg.r = r
}

func (tg *TcpGroup) InitPool() {
	tg.pool = NewConnPool(
		tg.Dst,
		tg.SrcIP,
		tg.MaxTcpConnPerIP,
		tg.TcpCreatThread,
		tg.TcpCreatRate,
		tg.TcpConnThread,
		tg.IsHttps,
		&tg.r.Receive,
		&tg.r.Send,
	)
	tg.pool.debug = tg.ctx.debug
	defer tg.ctx.wg.Done()
}
func (tg *TcpGroup) Run() {
	for i := 0; i < tg.ReqThread; i++ {
		tg.ctx.wg.Add(1)
		go tg.task()
	}
	defer tg.ctx.wg.Done()
}

func (tg *TcpGroup) task() {
	defer func() {
		tg.ctx.wg.Done()
		v := recover()
		if v != nil && v != sendOnCloseError {
			panic(v)
		}
	}()
	n := len(tg.sendHttpConfs)
	reqCount := 0
	conn := tg.pool.Get()
	for {
		select {
		case <-tg.ctx.ctx.Done():
			return
		default:
			if reqCount < tg.MaxReqest {
				tg.rl.Wait(tg.ctx.ctx)
				sendHttpBytes := tg.sendHttpConfs[reqCount%n].GetReqBytes()
				rr, err := tg.doReq(conn, sendHttpBytes)
				if err != nil {
					tg.r.WriteErr(err)
					tg.pool.Put(conn)
					conn = tg.pool.Get()
					reqCount = 0
					continue
				}
				reqCount++
				tg.r.maxResultChan <- rr
			} else {
				reqCount = 0
				tg.pool.Put(conn)
				conn = tg.pool.Get()
			}
		}
	}
}

func (tg *TcpGroup) doReq(conn net.Conn, httpByte []byte) (*ReqResult, error) {
	// start_time := time.Now()
	// timeout := 5 * time.Second
	// if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
	// 	return nil, err
	// }
	_, err := conn.Write(httpByte)
	if err != nil {
		return nil, err
	}
	// if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
	// 	return nil, err
	// }
	start_time := time.Now()
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return nil, err
	}
	// _, err = ioutil.ReadAll(resp.Body)
	// if err != nil {
	// 	return nil, err
	// }
	respTime := time.Since(start_time).Nanoseconds()
	respCode := resp.StatusCode
	rr := &ReqResult{
		code:    respCode,
		start:   start_time,
		reqtime: respTime,
	}
	defer resp.Body.Close()
	return rr, nil
}
