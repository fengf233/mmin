package perf

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultTimeout = 500 * time.Second
)

type TcpGroup struct {
	Name            string   `yaml:"Name"`
	MaxTcpConnPerIP int      `yaml:"MaxTcpConnPerIP"`
	TcpConnThread   int      `yaml:"TcpConnThread"`
	TcpCreatThread  int      `yaml:"TcpCreatThread"`
	TcpCreatRate    int      `yaml:"TcpCreatRate"`
	WriteTimeout    int      `yaml:"WriteTimeout"`
	ReadTimeout     int      `yaml:"ReadTimeout"`
	ConnTimeout     int      `yaml:"ConnTimeout"`
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
	writeTimeout  time.Duration
	readTimeout   time.Duration
	connTimeout   time.Duration
}

func (tg *TcpGroup) Init(ctx *RunCtx, r *Report, reqMap map[string]*HTTPconf) {
	if tg.WriteTimeout == 0 {
		tg.writeTimeout = defaultTimeout
	} else {
		tg.writeTimeout = time.Duration(tg.WriteTimeout) * time.Second
	}
	if tg.ReadTimeout == 0 {
		tg.readTimeout = defaultTimeout
	} else {
		tg.readTimeout = time.Duration(tg.ReadTimeout) * time.Second
	}
	if tg.connTimeout == 0 {
		tg.connTimeout = defaultTimeout
	} else {
		tg.connTimeout = time.Duration(tg.connTimeout) * time.Second
	}
	if tg.TcpConnThread == 0 {
		tg.TcpConnThread = tg.ReqThread/tg.MaxReqest + 1
	}
	if tg.TcpCreatThread == 0 {
		tg.TcpCreatThread = len(tg.SrcIP)/2 + 1
	}
	for _, sendHttpName := range tg.SendHttp {
		if httpConf := reqMap[sendHttpName]; httpConf != nil {
			if err := httpConf.SetReqBytes(); err != nil {
				log.Fatalf("TcpGroup httpConf init fail for %s: %v", sendHttpName, err)
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
		tg.connTimeout,
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
		if v := recover(); v != nil {
			if v == sendOnCloseError || strings.Contains(fmt.Sprint(v), "send on closed channel") {
				return
			}
			log.Printf("panic in task: %v", v)
			panic(v)
		}
	}()

	reqCount := 0
	conn := tg.pool.Get()
	httpConfCount := len(tg.sendHttpConfs)

	for {
		select {
		case <-tg.ctx.ctx.Done():
			return
		default:
			if reqCount < tg.MaxReqest {
				if err := tg.rl.Wait(tg.ctx.ctx); err != nil {
					continue
				}

				sendHttpBytes := tg.sendHttpConfs[reqCount%httpConfCount].GetReqBytes()
				rr, err := tg.doReq(conn, sendHttpBytes)

				if err != nil {
					tg.r.WriteErr(err)
					tg.pool.Put(conn)
					conn = tg.pool.Get()
					reqCount = 0
					continue
				}

				select {
				case tg.r.maxResultChan <- rr:
				default:
					return
				}

				reqCount++
			} else {
				reqCount = 0
				tg.pool.Put(conn)
				conn = tg.pool.Get()
			}
		}
	}
}

func (tg *TcpGroup) doReq(conn net.Conn, httpByte []byte) (*ReqResult, error) {
	start := time.Now()

	if err := conn.SetWriteDeadline(time.Now().Add(tg.writeTimeout)); err != nil {
		return nil, fmt.Errorf("set write deadline: %w", err)
	}

	if _, err := conn.Write(httpByte); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(tg.readTimeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	defer resp.Body.Close()

	respTime := time.Since(start).Nanoseconds()

	return &ReqResult{
		code:    resp.StatusCode,
		start:   start,
		reqtime: respTime,
	}, nil
}
