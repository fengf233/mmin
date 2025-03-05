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

	"github.com/gorilla/websocket"
)

type WebServer struct {
	isRunning int32
	report    *perf.Report
	clients   map[*websocket.Conn]bool
	upgrader  websocket.Upgrader
	runConf   *perf.RunConf
}

func NewWebServer() *WebServer {
	return &WebServer{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *WebServer) handleAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/test/start":
		s.handleStartTest(w, r)
	case "/api/test/stop":
		s.handleStopTest(w, r)
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
		http.Error(w, "Invalid configuration", http.StatusBadRequest)
		return
	}
	s.runConf = runConf
	atomic.StoreInt32(&s.isRunning, 1)

	go func() {
		s.report = runConf.Run()
		atomic.StoreInt32(&s.isRunning, 0)
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func (s *WebServer) handleStopTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if atomic.LoadInt32(&s.isRunning) == 0 {
		http.Error(w, "No test is running", http.StatusBadRequest)
		return
	}

	if s.runConf != nil {
		s.runConf.Stop()
		atomic.StoreInt32(&s.isRunning, 0)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func (s *WebServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	s.clients[conn] = true
	defer delete(s.clients, conn)

	// Keep connection alive and handle client messages
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (s *WebServer) broadcastReport(report *perf.Report) {
	data := map[string]interface{}{
		"qps":             report.Rate,
		"avgResponseTime": float32(report.AllReqTime) / float32(report.Success),
		"successRate":     float32(report.Success) * 100,
		"activeConns":     len(s.clients),
		"qpsData":         []int64{report.Rate},
		"responseTimeData": []float64{
			float64(report.ReqTime) / float64(report.Rate),
		},
		"statusCodeData": report.Respcode,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	for client := range s.clients {
		client.WriteMessage(websocket.TextMessage, jsonData)
	}
}

func StartWebServer(port string) error {
	server := NewWebServer()
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/", server.handleAPI)
	mux.HandleFunc("/ws", server.handleWebSocket)

	// 静态文件服务
	fs := web.GetFileSystem()
	mux.Handle("/", http.FileServer(fs))

	fmt.Printf("Web server starting on http://localhost:%s\n", port)
	return http.ListenAndServe(":"+port, mux)
}
