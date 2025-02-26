package perf

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/InVisionApp/tabular"
	"github.com/beorn7/perks/quantile"
	"gopkg.in/yaml.v2"
)

const (
	printInterval = time.Second
	maxRetries    = 5
)

// Report 性能测试报告结构
type Report struct {
	Success       int64          `yaml:"success"`    //总成功数
	Rate          int64          `yaml:"rate"`       //1秒内速率,实时速率
	Receive       int64          `yaml:"receive"`    //总收流量
	Send          int64          `yaml:"send"`       //总发流量
	ReqTime       float64        `yaml:"reqTime"`    //1秒内响应时间,实时速率
	AllReqTime    float64        `yaml:"allReqTime"` //总响应时间,用于计算平均响应时间
	AvgRate       float32        `yaml:"avgRate"`    //平均速率
	AvgReceive    float32        `yaml:"avgReceive"` //平均响应吞吐
	AvgSend       float32        `yaml:"avgSend"`    //平均发送吞吐
	StartTime     time.Time      `yaml:"start_time"`
	Respcode      map[int]int    `yaml:"respcode"`
	ErrMap        map[string]int `yaml:"errMap"`
	maxResultChan chan *ReqResult
	rwlock        *sync.RWMutex
	ctx           *RunCtx
	est           *quantile.Stream
}

// ReqResult 请求结果
type ReqResult struct {
	code    int
	start   time.Time
	reqtime int64
}

func NewReport(ctx *RunCtx, maxResult int) *Report {
	return &Report{
		Success:       0,
		Rate:          0,
		Receive:       0,
		Send:          0,
		AvgRate:       0,
		Respcode:      make(map[int]int),
		ErrMap:        make(map[string]int),
		maxResultChan: make(chan *ReqResult, maxResult),
		ctx:           ctx,
		rwlock:        &sync.RWMutex{},
		est:           quantile.NewTargeted(quantilesTarget),
	}
}

func (r *Report) WriteErr(err error) {
	if err == nil {
		return
	}
	r.rwlock.Lock()
	r.ErrMap[err.Error()]++
	r.rwlock.Unlock()
}

var quantiles = []float64{0.50, 0.75, 0.90, 0.95, 0.99}

var quantilesTarget = map[float64]float64{
	0.50: 0.01,
	0.75: 0.01,
	0.90: 0.001,
	0.95: 0.001,
	0.99: 0.001,
}

func (r *Report) Printer() {
	defer r.ctx.wg.Done()

	rowTab := r.createRowTable()
	rowFormat := rowTab.Print("*")

	lastPrintTime := time.Now()
	isStarted := false

	for {
		select {
		case <-r.ctx.ctx.Done():
			r.printFinalReport()
			return

		case result := <-r.maxResultChan:
			if !isStarted {
				r.initStartTime(result.start)
				isStarted = true
				lastPrintTime = result.start
				continue
			}

			r.updateStats(result)

			if result.start.Sub(lastPrintTime) >= printInterval {
				r.printProgress(rowFormat, result.start)
				lastPrintTime = result.start
			}
		default:
		}
	}
}

func (r *Report) createRowTable() *tabular.Table {
	tab := tabular.New()
	tab.Col("Time", "Time", 10)
	tab.Col("Success", "Success", 12)
	tab.Col("Rate", "Rate", 12)
	tab.Col("ReqTime", "ReqTime", 12)
	tab.Col("Send", "Send", 12)
	tab.Col("Receive", "Receive", 12)
	tab.Col("Status", "Status", 20)
	return &tab
}

func (r *Report) createSumTable() *tabular.Table {
	tab := tabular.New()
	tab.Col("Result", "Result", 10)
	tab.Col("Statistics", "Statistics", 85)
	return &tab
}

func (r *Report) initStartTime(t time.Time) {
	r.rwlock.Lock()
	r.StartTime = t
	r.rwlock.Unlock()
}

func (r *Report) updateStats(result *ReqResult) {
	atomic.AddInt64(&r.Success, 1)
	atomic.AddInt64(&r.Rate, 1)
	r.rwlock.Lock()
	r.Respcode[result.code]++
	r.ReqTime += float64(result.reqtime) / 1e6
	r.AllReqTime += float64(result.reqtime) / 1e6
	r.est.Insert(float64(result.reqtime) / 1e6)
	r.rwlock.Unlock()
}

func (r *Report) printProgress(format string, now time.Time) {
	receive := atomic.LoadInt64(&r.Receive)
	send := atomic.LoadInt64(&r.Send)
	runtime := now.Sub(r.StartTime).Seconds()

	r.rwlock.RLock()
	status := r.formatStatus()
	reqTimeMs := float32(r.ReqTime) / float32(r.Rate)
	r.rwlock.RUnlock()

	rMbps := float32(receive) * 8 / 1000 / 1000 / float32(runtime)
	sMbps := float32(send) * 8 / 1000 / 1000 / float32(runtime)

	fmt.Printf(format,
		float32(time.Since(r.StartTime).Seconds()),
		r.Success, r.Rate, reqTimeMs, sMbps, rMbps, status)

	if r.ctx.debug {
		r.printErrors()
	}

	atomic.StoreInt64(&r.Rate, 0)
	r.rwlock.Lock()
	r.ReqTime = 0
	r.rwlock.Unlock()
}

func (r *Report) formatStatus() string {
	var status string
	for k, v := range r.Respcode {
		status += "[" + strconv.Itoa(k) + "]" + ":" + strconv.Itoa(v)
	}
	return status
}

func (r *Report) printErrors() {
	r.rwlock.RLock()
	for errK, errY := range r.ErrMap {
		fmt.Println(errK + ":" + strconv.Itoa(errY))
	}
	r.rwlock.RUnlock()
}

func (r *Report) printFinalReport() {
	sumTab := r.createSumTable()
	sumFormat := sumTab.Print("*")

	runtime := time.Now().Sub(r.StartTime).Seconds()
	r.AvgRate = float32(float64(r.Success) / runtime)
	r.AvgReceive = float32(float64(r.Receive) * 8.0 / 1000.0 / 1000.0 / runtime)
	r.AvgSend = float32(float64(r.Send) * 8.0 / 1000.0 / 1000.0 / runtime)
	fmt.Println("")
	fmt.Printf(sumFormat, "RunTime:", fmt.Sprintf("%f s", runtime))
	fmt.Printf(sumFormat, "Success:", r.Success)
	fmt.Printf(sumFormat, "AvgRate:", fmt.Sprintf("%f Req/s", r.AvgRate))
	fmt.Printf(sumFormat, "ReqTime:", fmt.Sprintf("%f ms", float32(r.AllReqTime)/float32(r.Success)))
	fmt.Printf(sumFormat, "Send:", fmt.Sprintf("%f Mbps", r.AvgSend))
	fmt.Printf(sumFormat, "Receive:", fmt.Sprintf("%f Mbps", r.AvgReceive))
	fmt.Printf(sumFormat, "Status:", r.formatStatus())
	var reqTimeQuantiles string
	for _, quantile := range quantiles {
		values := r.est.Query(quantile)
		t_str := fmt.Sprintf("%d: %f ", int(quantile*100), values)
		reqTimeQuantiles += t_str
	}
	fmt.Printf(sumFormat, "ReqTime Quantile:", reqTimeQuantiles)
	close(r.maxResultChan)
}

func (r *Report) RemotePrinter(remoteDst string) {
	defer r.ctx.wg.Done()
	tryTimes := 0
	for {
		select {
		case <-r.ctx.ctx.Done():
			return
		default:
			time.Sleep(1 * time.Second)
			if tryTimes > 5 {
				fmt.Println(remoteDst, "GetRemoteReport fail")
			}
			statu, Rr := GetRemoteReport(remoteDst)
			if statu == "Stop" {
				if Rr == nil {
					tryTimes++
					continue
				}
				r.rwlock.Lock()
				r.Success += Rr.Success
				r.AvgRate += Rr.AvgRate
				r.AllReqTime += Rr.AllReqTime
				r.AvgSend += Rr.AvgSend
				r.AvgReceive += Rr.AvgReceive
				for k, v := range Rr.Respcode {
					r.Respcode[k] += v
				}
				for k, v := range Rr.ErrMap {
					r.ErrMap[k] += v
				}
				r.rwlock.Unlock()
				return
			}
		}
	}
}

func (r *Report) RemotePrintResult() {
	var Status string
	for k, v := range r.Respcode {
		Status = Status + "[" + strconv.Itoa(k) + "]" + ":" + strconv.Itoa(v)
	}
	fmt.Println(" Result: \n",
		" Success: ", r.Success, "\n",
		" AvgRate: ", r.AvgRate, "\n",
		" ReqTime: ", float32(r.AllReqTime)/float32(r.Success), "\n",
		" Send:", r.AvgSend, "Mbps\n",
		" Receive:", r.AvgReceive, "Mbps\n",
		" Status:", Status)
}

func GetRemoteReport(remoteDst string) (string, *Report) {
	req, err := http.NewRequest("GET", "http://"+remoteDst+"/report", nil)
	if err != nil {
		fmt.Println("GetRemoteReport:", remoteDst, "err:", err.Error())
		return "Error", nil
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("GetRemoteReport:", remoteDst, "err:", err.Error())
		return "Error", nil
	}
	respbody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("GetRemoteReport:", remoteDst, "err:", err.Error())
		return "Error", nil
	}
	if string(respbody) == "Running" {
		fmt.Println("GetRemoteReport:", remoteDst, " Running")
		return "Running", nil
	} else if string(respbody) == "Error" {
		fmt.Println("GetRemoteReport:", remoteDst, " Error")
	} else {
		var report *Report
		err := yaml.Unmarshal(respbody, &report)
		if err != nil {
			fmt.Println("GetRemoteReport:", remoteDst, "yaml.Unmarshal error", err.Error())
		}
		return "Stop", report
	}
	defer resp.Body.Close()
	return "Error", nil
}
