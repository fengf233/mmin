package perf

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/InVisionApp/tabular"
	"golang.org/x/time/rate"
)

const (
	defaultReadTimeout = 10 * time.Millisecond
	defaultDialTimeout = 5 * time.Second
)

type MyConn struct {
	net.Conn
	r, w   *int64
	dialer *net.Dialer
}

// Read wraps the underlying connection's Read method and tracks bytes read
func (c *MyConn) Read(b []byte) (n int, err error) {
	sz, err := c.Conn.Read(b)
	if err == nil && sz > 0 {
		atomic.AddInt64(c.r, int64(sz))
	}
	return sz, err
}

// Write wraps the underlying connection's Write method and tracks bytes written
func (c *MyConn) Write(b []byte) (n int, err error) {
	sz, err := c.Conn.Write(b)
	if err == nil && sz > 0 {
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
	debug       bool

	stats struct {
		active  int32
		failed  int32
		created int32
	}

	closed    int32         // 添加关闭状态标志
	closeCh   chan struct{} // 用于通知关闭的channel
	closeOnce sync.Once     // 确保只关闭一次
}

// 创建连接池
func NewConnPool(dst string, srcIP []string, maxConnPerIP int, creatThread int, creatRate int, connThread int, isHttps bool, r *int64, w *int64, connTimeout time.Duration) *ConnPool {
	srcIPLen := max(1, len(srcIP))
	maxConn := srcIPLen * maxConnPerIP
	atomic.AddInt32(&allPoolMaxConn, int32(maxConn))
	connsChan := make(chan *MyConn, maxConn)
	factoryChan := make(chan *MyConn, maxConn)
	ctx, cancel := context.WithCancel(context.Background())
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // 跳过证书验证
	}
	var rl *rate.Limiter = nil
	if creatRate > 0 {
		rl = rate.NewLimiter(rate.Limit(creatRate), 1)
	}

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
		closed:      0,
		closeCh:     make(chan struct{}),
		closeOnce:   sync.Once{},
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

	dialer := &net.Dialer{
		Timeout: defaultDialTimeout,
	}

	if srcip != "" {
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(srcip)}
	}

	for created := 0; created < maxConnPerIP; created++ {
		if pool.rl != nil {
			if err := pool.rl.Wait(pool.ctx); err != nil {
				return // context cancelled
			}
		}

		conn, err := pool.getConn(dialer)
		if err != nil {
			atomic.AddInt32(&pool.stats.failed, 1)
			if pool.debug {
				poolErrMu.Lock()
				poolErr[err.Error()]++
				poolErrMu.Unlock()
			}
			continue
		}

		myconn := &MyConn{
			dialer: dialer,
			Conn:   conn,
			r:      pool.r,
			w:      pool.w,
		}

		select {
		case pool.connsChan <- myconn:
			atomic.AddInt32(&pool.stats.created, 1)
			atomic.AddInt32(&pool.stats.active, 1)
			atomic.AddInt32(&poolConnCount, 1)
		case <-pool.ctx.Done():
			myconn.Close()
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
	myconn := <-pool.connsChan
	return myconn
}

// 获取连接
func (pool *ConnPool) GetWithoutClose() *MyConn {
	for {
		select {
		case <-pool.ctx.Done():
			return nil
		case conn := <-pool.connsChan:
			if conn == nil {
				continue
			}

			// Check connection health
			one := make([]byte, 1)
			if err := conn.SetReadDeadline(time.Now().Add(defaultReadTimeout)); err != nil {
				pool.Put(conn)
				continue
			}

			if _, err := conn.Read(one); err == io.EOF {
				pool.Put(conn)
				continue
			}

			// Reset read deadline
			if err := conn.SetReadDeadline(time.Time{}); err != nil {
				pool.Put(conn)
				continue
			}

			return conn
		}
	}
}

func (pool *ConnPool) factory() {
	defer pool.wg.Done()
	defer func() {
		if r := recover(); r != nil && r != sendOnCloseError {
			log.Printf("Panic in connection factory: %v", r)
			panic(r)
		}
	}()

	for {
		select {
		case <-pool.ctx.Done():
			return
		case conn, ok := <-pool.factoryChan:
			if !ok {
				return
			}

			conn.Close()
			atomic.AddInt32(&pool.stats.active, -1)

			newConn, err := pool.getConn(conn.dialer)
			if err != nil {
				pool.factoryChan <- conn // retry
				continue
			}

			conn.Conn = newConn
			atomic.AddInt32(&pool.stats.active, 1)
			pool.connsChan <- conn
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
	pool.closeOnce.Do(func() {
		atomic.StoreInt32(&pool.closed, 1)

		// 先取消上下文
		pool.cancel()

		// 关闭通知channel
		close(pool.closeCh)

		// 关闭连接相关channel
		close(pool.connsChan)
		close(pool.factoryChan)

		// 关闭所有连接
		for conn := range pool.connsChan {
			if conn != nil {
				conn.Close()
			}
		}
	})
}

func (p *ConnPool) IsClosed() bool {
	return atomic.LoadInt32(&p.closed) == 1
}
