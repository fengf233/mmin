package perf

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/InVisionApp/tabular"
	"golang.org/x/time/rate"
)

type MyConn struct {
	net.Conn
	r, w   *int64
	dialer *net.Dialer
}

func (c *MyConn) Read(b []byte) (n int, err error) {
	sz, err := c.Conn.Read(b)

	if err == nil {
		atomic.AddInt64(c.r, int64(sz))
	}
	return sz, err
}

func (c *MyConn) Write(b []byte) (n int, err error) {
	sz, err := c.Conn.Write(b)

	if err == nil {
		atomic.AddInt64(c.w, int64(sz))
	}
	return sz, err
}

type ConnPool struct {
	dst          string
	srcIP        []string
	maxConnPerIP int
	creatThread  int
	creatRate    int
	connThread   int
	isHttps      bool
	r, w         *int64

	maxConn     int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	rl          *rate.Limiter
	tlsConfig   *tls.Config
	connsChan   chan *MyConn
	factoryChan chan *MyConn
}

// 创建连接池
func NewConnPool(dst string, srcIP []string, maxConnPerIP int, creatThread int, creatRate int, connThread int, isHttps bool, r *int64, w *int64) *ConnPool {
	srcIPLen := 1
	if len(srcIP) != 0 {
		srcIPLen = len(srcIP)
	}
	maxConn := srcIPLen * maxConnPerIP
	atomic.AddInt32(&allPoolMaxConn, int32(maxConn))
	connsChan := make(chan *MyConn, maxConn)
	factoryChan := make(chan *MyConn, maxConn)
	ctx, cancel := context.WithCancel(context.Background())
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // 跳过证书验证
	}
	rl := rate.NewLimiter(rate.Limit(creatRate), 1)

	pool := &ConnPool{
		dst:          dst,
		srcIP:        srcIP,
		maxConnPerIP: maxConnPerIP,
		creatThread:  creatThread,
		creatRate:    creatRate,
		connThread:   connThread,
		isHttps:      isHttps,
		r:            r,
		w:            w,

		maxConn:     maxConn,
		ctx:         ctx,
		cancel:      cancel,
		rl:          rl,
		tlsConfig:   tlsConfig,
		connsChan:   connsChan,
		factoryChan: factoryChan,
	}
	pool.init()
	return pool
}

func (pool *ConnPool) init() {
	pool.creatConns()
	for i := 0; i < pool.connThread; i++ {
		pool.wg.Add(1)
		go pool.factory()
	}
}

func (pool *ConnPool) creatConns() {
	maxCreateConns := 0
	if len(pool.srcIP) != 0 {
		nsrcip := len(pool.srcIP)
		threadPerSrcIP := pool.creatThread / nsrcip
		if threadPerSrcIP == 0 {
			threadPerSrcIP = 1
		}
		creatConnsNum := pool.maxConnPerIP / threadPerSrcIP
		for _, srcip := range pool.srcIP {
			for i := 0; i < threadPerSrcIP; i++ {
				pool.wg.Add(1)
				maxCreateConns += creatConnsNum
				go pool.creat(srcip, creatConnsNum)
			}
		}
	} else {
		creatConnsNum := pool.maxConnPerIP / pool.creatThread
		for i := 0; i < pool.creatThread; i++ {
			pool.wg.Add(1)
			maxCreateConns += creatConnsNum
			go pool.creat("", pool.maxConnPerIP/pool.creatThread)
		}
	}
	lastConnsNum := pool.maxConn - maxCreateConns
	if lastConnsNum != 0 {
		pool.wg.Add(1)
		go pool.creat("", lastConnsNum)
	}
	pool.wg.Wait()
}

var poolErr map[string]int = map[string]int{}
var poolErrMu sync.RWMutex
var poolConnCount int32
var allPoolMaxConn int32

func PoolPrint(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Creat TCP conns Start:")
	var poolformat string
	rowTab := tabular.New()
	rowTab.Col("Time", "Time", 10)
	rowTab.Col("ConnCount", "ConnCount", 12)
	poolformat = rowTab.Print("*")
	timeSec := 1
	for {
		time.Sleep(1 * time.Second)
		if poolConnCount < allPoolMaxConn {
			var errStr string
			for errK, errV := range poolErr {
				errStr = errK + ":" + strconv.Itoa(errV) + "\n"
			}
			fmt.Printf(poolformat, timeSec, poolConnCount)
			if errStr != "" {
				fmt.Print(errStr)
			}
			timeSec++
		} else {
			fmt.Println("Creat TCP conns:", poolConnCount)
			fmt.Println("")
			return
		}
	}
}

func (pool *ConnPool) creat(srcip string, maxConnPerIP int) {
	defer pool.wg.Done()
	maxConnPerIPCount := 0
	for {
		if maxConnPerIPCount < maxConnPerIP {
			pool.rl.Wait(pool.ctx)
			var dialer net.Dialer
			var conn net.Conn
			if srcip != "" {
				dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(srcip)}
			}
			conn, err := pool.getConn(&dialer)
			if err != nil {
				poolErrMu.Lock()
				poolErr[err.Error()] += 1
				poolErrMu.Unlock()
				// time.Sleep(1 * time.Second)
				continue
			}
			myconn := &MyConn{
				dialer: &dialer,
				Conn:   conn,
				r:      pool.r,
				w:      pool.w,
			}
			pool.connsChan <- myconn
			maxConnPerIPCount += 1
			atomic.AddInt32(&poolConnCount, 1)
		} else {
			return
		}
	}
}

func (pool *ConnPool) getConn(dialer *net.Dialer) (net.Conn, error) {
	if pool.isHttps {
		conn, err := tls.DialWithDialer(dialer, "tcp", pool.dst, pool.tlsConfig)
		if err != nil {
			// fmt.Println("tls.DialWithDialer:", err.Error())
			return nil, err
		}
		return conn, nil
	} else {
		conn, err := dialer.Dial("tcp", pool.dst)
		if err != nil {
			// fmt.Println("dialer.Dial:", err.Error())
			return nil, err
		}
		return conn, nil
	}
}

func (pool *ConnPool) Get() *MyConn {
	var myconn *MyConn
	myconn = <-pool.connsChan
	return myconn
}

// 获取连接
func (pool *ConnPool) GetWithoutClose() *MyConn {
	var myconn *MyConn
	for {
		select {
		case <-pool.ctx.Done():
			return nil
		case myconn = <-pool.connsChan:
			//判断连接是否断开
			one := make([]byte, 1)
			myconn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
			if _, err := myconn.Read(one); err == io.EOF {
				pool.Put(myconn)
				continue
			} else {
				var zero time.Time
				myconn.SetReadDeadline(zero)
				return myconn
			}
		}
	}
}

func (pool *ConnPool) factory() {
	defer func() {
		pool.wg.Done()
		v := recover()
		if v != nil && v != sendOnCloseError {
			panic(v)
		}
	}()
	for {
		select {
		case <-pool.ctx.Done():
			return
		case myconn, ok := <-pool.factoryChan:
			if !ok {
				return
			}
			myconn.Conn.Close()
			newconn, err := pool.getConn(myconn.dialer)
			if err != nil {
				// fmt.Println("pool factory", err.Error())
				pool.factoryChan <- myconn
				continue
			}
			myconn.Conn = newconn
			pool.connsChan <- myconn
		}
	}
}

// 释放连接
func (pool *ConnPool) Put(myconn *MyConn) {
	pool.factoryChan <- myconn
}

func (pool *ConnPool) Len() int {
	return len(pool.connsChan)
}

// 关闭连接池
func (pool *ConnPool) Close() {
	pool.cancel()
	// pool.wg.Wait()
	close(pool.connsChan)
	close(pool.factoryChan)
	for myconn1 := range pool.connsChan {
		myconn1.Close()
		if len(pool.connsChan) == 0 {
			break
		}
	}
	for myconn2 := range pool.factoryChan {
		myconn2.Close()
		if len(pool.factoryChan) == 0 {
			break
		}
	}
}
