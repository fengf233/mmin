package perf

import (
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
	ctx         *RunCtx
	rl          *rate.Limiter
	tlsConfig   *tls.Config
	connsChan   chan *MyConn
	factoryChan chan *MyConn
	pool_wg     *sync.WaitGroup
	closed      int32         // 添加关闭状态标志
	closeCh     chan struct{} // 用于通知关闭的channel
	closeOnce   sync.Once     // 确保只关闭一次
}

// 创建连接池
func NewConnPool(dst string, srcIP []string, maxConnPerIP int, creatThread int, creatRate int, connThread int, isHttps bool, r *int64, w *int64, connTimeout time.Duration, runCtx *RunCtx) *ConnPool {
	srcIPLen := max(1, len(srcIP))
	maxConn := srcIPLen * maxConnPerIP
	atomic.AddInt32(&allPoolMaxConn, int32(maxConn))
	connsChan := make(chan *MyConn, maxConn)
	factoryChan := make(chan *MyConn, maxConn)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // 跳过证书验证
	}
	var rl *rate.Limiter = nil
	if creatRate > 0 {
		rl = rate.NewLimiter(rate.Limit(creatRate), 1)
	}
	pool_wg := &sync.WaitGroup{}

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
		ctx:         runCtx,
		rl:          rl,
		tlsConfig:   tlsConfig,
		connsChan:   connsChan,
		factoryChan: factoryChan,
		closed:      0,
		closeCh:     make(chan struct{}),
		closeOnce:   sync.Once{},
		pool_wg:     pool_wg,
	}
	pool.init()
	return pool
}

func (pool *ConnPool) init() {
	pool.creatConns()
	for i := 0; i < pool.connThread; i++ {
		// 在ctx上等待
		pool.ctx.wg.Add(1)
		go func() {
			defer pool.ctx.wg.Done()
			pool.factory()
		}()
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
				maxCreateConns += creatConnsNum
				pool.pool_wg.Add(1)
				go func() {
					defer pool.pool_wg.Done()
					pool.creat(srcip, creatConnsNum)
				}()
			}
		}
	} else {
		creatConnsNum := pool.maxConnPerIP / pool.creatThread
		for i := 0; i < pool.creatThread; i++ {
			maxCreateConns += creatConnsNum
			pool.pool_wg.Add(1)
			go func() {
				defer pool.pool_wg.Done()
				pool.creat("", pool.maxConnPerIP/pool.creatThread)
			}()
		}
	}
	lastConnsNum := pool.maxConn - maxCreateConns
	if lastConnsNum != 0 {
		pool.pool_wg.Add(1)
		go func() {
			defer pool.pool_wg.Done()
			pool.creat("", lastConnsNum)
		}()
	}
}

var (
	poolErr         map[string]int
	poolErrMu       sync.RWMutex
	ActiveConnCount int32
	FailedConnCount int32
	allPoolMaxConn  int32
)

func PoolGlobalInit() {
	poolErr = make(map[string]int)
	ActiveConnCount = 0
	FailedConnCount = 0
	allPoolMaxConn = 0
}

func PoolPrint(ctx *RunCtx) {
	fmt.Println("Creat TCP conns Start:")
	var poolformat string
	rowTab := tabular.New()
	rowTab.Col("Time", "Time", 10)
	rowTab.Col("ConnCount", "ConnCount", 12)
	poolformat = rowTab.Print("*")
	timeSec := 1
	for {
		select {
		case <-ctx.ctx.Done():
			return
		default:
			time.Sleep(1 * time.Second)
			if ActiveConnCount < allPoolMaxConn {
				var errStr string
				for errK, errV := range poolErr {
					errStr = errK + ":" + strconv.Itoa(errV) + "\n"
				}
				fmt.Printf(poolformat, timeSec, ActiveConnCount)
				if errStr != "" {
					fmt.Print(errStr)
				}
				timeSec++
			} else {
				fmt.Println("Creat TCP conns:", ActiveConnCount)
				fmt.Println("")
				return
			}
		}
	}
}

func (pool *ConnPool) creat(srcip string, maxConnPerIP int) {
	dialer := &net.Dialer{
		Timeout: defaultDialTimeout,
	}

	if srcip != "" {
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(srcip)}
	}

	for created := 0; created < maxConnPerIP; created++ {
		if pool.rl != nil {
			if err := pool.rl.Wait(pool.ctx.ctx); err != nil {
				return // context cancelled
			}
		}

		conn, err := pool.getConn(dialer)
		if err != nil {
			atomic.AddInt32(&FailedConnCount, 1)
			if pool.ctx.debug {
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
			atomic.AddInt32(&ActiveConnCount, 1)
		case <-pool.ctx.ctx.Done():
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
	select {
	case <-pool.ctx.ctx.Done():
		return nil
	case myconn := <-pool.connsChan:
		return myconn
	}
}

// 获取连接
func (pool *ConnPool) GetWithoutClose() *MyConn {
	for {
		select {
		case <-pool.ctx.ctx.Done():
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
	defer func() {
		if r := recover(); r != nil && r != sendOnCloseError {
			log.Printf("Panic in connection factory: %v", r)
			panic(r)
		}
	}()

	for {
		select {
		case <-pool.ctx.ctx.Done():
			return
		case myconn, ok := <-pool.factoryChan:
			if !ok {
				return
			}

			myconn.Close()
			atomic.AddInt32(&ActiveConnCount, -1)
			newConn, err := pool.getConn(myconn.dialer)
			if err != nil {
				pool.factoryChan <- myconn // retry
				continue
			}

			myconn.Conn = newConn
			atomic.AddInt32(&ActiveConnCount, 1)
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
	pool.closeOnce.Do(func() {
		atomic.StoreInt32(&pool.closed, 1)
		pool.pool_wg.Wait()
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

// validate 验证TCP组配置
func (tg *TcpGroup) validate() error {
	if tg.Name == "" {
		return fmt.Errorf("组名不能为空")
	}
	if tg.Dst == "" {
		return fmt.Errorf("目标地址不能为空")
	}
	if tg.MaxTcpConnPerIP <= 0 {
		return fmt.Errorf("每IP最大连接数必须大于0")
	}
	if tg.ReqThread <= 0 {
		return fmt.Errorf("请求线程数必须大于0")
	}
	if tg.MaxReqest <= 0 {
		return fmt.Errorf("每TCP最大请求数必须大于0")
	}
	if len(tg.SendHttp) == 0 {
		return fmt.Errorf("HTTP请求列表不能为空")
	}
	return nil
}
