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
}

//每个请求的结果
type ReqResult struct {
	code    int
	start   time.Time
	reqtime int64
}

func NewReport(ctx *RunCtx, maxResult int) *Report {
	r := &Report{
		Success:       0,
		Rate:          0,
		Receive:       0,
		Send:          0,
		AvgRate:       0,
		Respcode:      map[int]int{},
		ErrMap:        map[string]int{},
		maxResultChan: make(chan *ReqResult, maxResult),
		ctx:           ctx,
	}
	var rwlock sync.RWMutex
	r.rwlock = &rwlock
	return r
}

func (r *Report) WriteErr(err error) {
	r.rwlock.Lock()
	r.ErrMap[err.Error()] += 1
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
	is_start := false
	tmp_start_time := time.Now()
	var rowTabformat string
	rowTab := tabular.New()
	rowTab.Col("Time", "Time", 10)
	rowTab.Col("Success", "Success", 12)
	rowTab.Col("Rate", "Rate", 12)
	rowTab.Col("ReqTime", "ReqTime", 12)
	rowTab.Col("Send", "Send", 12)
	rowTab.Col("Receive", "Receive", 12)
	rowTab.Col("Status", "Status", 20)
	var sumTabformat string
	sumTab := tabular.New()
	sumTab.Col("Result", "Result", 10)
	sumTab.Col("Statistics", "Statistics", 85)

	est := quantile.NewTargeted(quantilesTarget)
	for {
		select {
		case <-r.ctx.ctx.Done():
			if len(r.maxResultChan) != 0 {
				for rr := range r.maxResultChan {
					r.Success += 1
					r.Rate += 1
					r.Respcode[rr.code] += 1
					r.ReqTime += float64(rr.reqtime) / 1e6
					r.AllReqTime += float64(rr.reqtime) / 1e6
					if len(r.maxResultChan) == 0 {
						break
					}
				}
			}
			var Status string
			for k, v := range r.Respcode {
				Status = Status + "[" + strconv.Itoa(k) + "]" + ":" + strconv.Itoa(v)
			}
			runtime := time.Now().Sub(r.StartTime).Seconds()
			r.AvgRate = float32(float64(r.Success) / runtime)
			r.AvgReceive = float32(float64(r.Receive) * 8.0 / 1024.0 / 1024.0 / runtime)
			r.AvgSend = float32(float64(r.Send) * 8.0 / 1024.0 / 1024.0 / runtime)
			fmt.Println("")
			sumTabformat = sumTab.Print("*")
			fmt.Printf(sumTabformat, "RunTime:", fmt.Sprintf("%f s", runtime))
			fmt.Printf(sumTabformat, "Success:", r.Success)
			fmt.Printf(sumTabformat, "AvgRate:", fmt.Sprintf("%f Req/s", r.AvgRate))
			fmt.Printf(sumTabformat, "ReqTime:", fmt.Sprintf("%f ms", float32(r.AllReqTime)/float32(r.Success)))
			fmt.Printf(sumTabformat, "Send:", fmt.Sprintf("%f Mbps", r.AvgSend))
			fmt.Printf(sumTabformat, "Receive:", fmt.Sprintf("%f Mbps", r.AvgReceive))
			fmt.Printf(sumTabformat, "Status:", Status)
			var reqTimeQuantiles string
			for _, quantile := range quantiles {
				values := est.Query(quantile)
				t_str := fmt.Sprintf("%d: %f ", int(quantile*100), values)
				reqTimeQuantiles += t_str
			}
			fmt.Printf(sumTabformat, "ReqTime Quantile:", reqTimeQuantiles)
			close(r.maxResultChan)
			return
		case rr := <-r.maxResultChan:
			if is_start == false {
				tmp_start_time = rr.start
				r.rwlock.Lock()
				r.StartTime = rr.start
				r.rwlock.Unlock()
				is_start = true
				rowTabformat = rowTab.Print("*")
			}
			if rr.start.Sub(tmp_start_time).Milliseconds() > 1000 {
				receive := atomic.LoadInt64(&r.Receive)
				send := atomic.LoadInt64(&r.Send)
				runtime := rr.start.Sub(r.StartTime).Seconds()
				RMbps := float32(receive) * 8 / 1024 / 1024 / float32(runtime)
				SMbps := float32(send) * 8 / 1024 / 1024 / float32(runtime)
				ReqTimeMs := float32(r.ReqTime) / float32(r.Rate)
				r.AvgRate = float32(float64(r.Success) / runtime)
				var Status string
				for k, v := range r.Respcode {
					Status = Status + "[" + strconv.Itoa(k) + "]" + ":" + strconv.Itoa(v)
				}
				errString := ""
				if r.ctx.debug {
					r.rwlock.RLock()
					for errK, errY := range r.ErrMap {
						errString = errString + errK + ":" + strconv.Itoa(errY) + "\n"
					}
					r.rwlock.RUnlock()
					fmt.Print(errString)
				}
				fmt.Printf(rowTabformat, float32(time.Since(r.StartTime).Seconds()), r.Success, r.Rate, ReqTimeMs, SMbps, RMbps, Status)
				tmp_start_time = rr.start
				r.Rate = 0
				r.ReqTime = 0
			}
			r.Success += 1
			r.Rate += 1
			r.Respcode[rr.code] += 1
			r.ReqTime += float64(rr.reqtime) / 1e6
			est.Insert(float64(rr.reqtime) / 1e6)
			r.AllReqTime += float64(rr.reqtime) / 1e6
		default:
		}
	}
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
