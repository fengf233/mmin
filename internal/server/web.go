package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mmin/internal/perf"
	"mmin/web"
	"net/http"
	"sync/atomic"
	"time"
)

type WebServer struct {
	isRunning int32
	runConf   *perf.RunConf
}

// 添加一个新的结构体来存储最终测试结果
type TestResult struct {
	Duration        float64     `json:"duration"`
	TotalRequests   int64       `json:"totalRequests"`
	SuccessRate     float64     `json:"successRate"`
	AvgQPS          float64     `json:"avgQps"`
	AvgResponseTime float64     `json:"avgResponseTime"`
	Send            float64     `json:"send"`
	Receive         float64     `json:"receive"`
	TotalTraffic    float64     `json:"totalTraffic"`
	StatusCodes     map[int]int `json:"statusCodes"`
}

func NewWebServer() *WebServer {
	return &WebServer{}
}

func (s *WebServer) handleAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/test/start":
		s.handleStartTest(w, r)
	case "/api/test/stop":
		s.handleStopTest(w, r)
	case "/api/test/status":
		s.handleTestStatus(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *WebServer) handleStartTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if atomic.LoadInt32(&s.isRunning) > 0 {
		http.Error(w, "Test is already running", http.StatusConflict)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}
	log.Printf("Received test configuration: %s", string(body))
	runConf, err := perf.ReadRunConfByByte(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid configuration: %v", err), http.StatusBadRequest)
		return
	}

	// 添加配置验证
	if err := runConf.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Configuration validation failed: %v", err), http.StatusBadRequest)
		return
	}

	s.runConf = runConf
	atomic.StoreInt32(&s.isRunning, 1)

	go func() {
		s.runConf.Run()
		atomic.StoreInt32(&s.isRunning, 0)
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *WebServer) getFinshTestResult() TestResult {
	// 生成测试结果
	runtime := s.runConf.Report.RunTime
	result := TestResult{
		Duration:        runtime,
		TotalRequests:   s.runConf.Report.Success,
		SuccessRate:     float64(s.runConf.Report.Respcode[200]) / float64(s.runConf.Report.Success) * 100,
		AvgQPS:          float64(s.runConf.Report.Success) / float64(runtime),
		AvgResponseTime: float64(s.runConf.Report.AllReqTime) / float64(s.runConf.Report.Success),
		Send:            float64(s.runConf.Report.Send) * 8 / 1000 / 1000 / float64(runtime),
		Receive:         float64(s.runConf.Report.Receive) * 8 / 1000 / 1000 / float64(runtime),
		TotalTraffic:    float64(s.runConf.Report.Send+s.runConf.Report.Receive) * 8 / 1000 / 1000 / float64(runtime),
		StatusCodes:     s.runConf.Report.Respcode,
	}
	return result
}

func (s *WebServer) handleStopTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if atomic.LoadInt32(&s.isRunning) == 0 {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "stopped", "msg": "no test running"})
		return
	}

	if s.runConf != nil {
		s.runConf.Stop()
		atomic.StoreInt32(&s.isRunning, 0)

		result := s.getFinshTestResult()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "stopped",
			"result": result,
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func (s *WebServer) handleTestStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	isRunning := atomic.LoadInt32(&s.isRunning) > 0
	data := map[string]interface{}{
		"running": isRunning,
	}
	data["activeConnCount"] = perf.ActiveConnCount
	data["failedConnCount"] = perf.FailedConnCount
	if !isRunning && s.runConf != nil {
		data["result"] = s.getFinshTestResult()
	}

	if s.runConf != nil && s.runConf.Report != nil && isRunning && s.runConf.Report.RunTime > 0 {
		// 准备图表数据
		// 转换状态码数据为图表格式
		statusCodeData := make([]interface{}, 0)
		for code, count := range s.runConf.Report.Respcode {
			statusCodeData = append(statusCodeData, []interface{}{
				fmt.Sprintf("%d", code),
				count,
			})
		}

		runtime := s.runConf.Report.RunTime
		// 准备流量数据 (转换为MB/s)
		trafficData := []interface{}{
			time.Now().Format("15:04:05"),
			float64(s.runConf.Report.Send) * 8 / 1000 / 1000 / float64(runtime),    // 发送流量
			float64(s.runConf.Report.Receive) * 8 / 1000 / 1000 / float64(runtime), // 接收流量
		}

		// 添加报告数据
		data["avgQps"] = float32(s.runConf.Report.Success) / float32(runtime)
		qpsData := []interface{}{
			time.Now().Format("15:04:05"),
			data["avgQps"],
		}
		data["avgResponseTime"] = float32(s.runConf.Report.AllReqTime) / float32(s.runConf.Report.Success)
		responseTimeData := []interface{}{
			time.Now().Format("15:04:05"),
			data["avgResponseTime"],
		}
		data["successRate"] = float32(s.runConf.Report.Respcode[200]) / float32(s.runConf.Report.Success) * 100
		data["qpsData"] = qpsData
		data["responseTimeData"] = responseTimeData
		data["statusCodeData"] = statusCodeData
		data["trafficData"] = trafficData
		data["send"] = float32(s.runConf.Report.Send) * 8 / 1000 / 1000 / float32(runtime)
		data["receive"] = float32(s.runConf.Report.Receive) * 8 / 1000 / 1000 / float32(runtime)
		data["duration"] = runtime
		data["successCount"] = s.runConf.Report.Success
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func StartWebServer(port string) error {
	server := NewWebServer()
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/", server.handleAPI)

	// 静态文件服务
	fs := web.GetFileSystem()
	mux.Handle("/", http.FileServer(fs))

	fmt.Printf("Web server starting on http://localhost:%s\n", port)
	return http.ListenAndServe(":"+port, mux)
}
