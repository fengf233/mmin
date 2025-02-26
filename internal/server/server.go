package server

import (
	"fmt"
	"io"
	"mmin/internal/perf"
	"net/http"
	"sync/atomic"

	"gopkg.in/yaml.v2"
)

const (
	contentType    = "application/x-yaml"
	maxRequestSize = 10 << 20 // 10MB
	serverRunning  = "running"
	serverStarted  = "start"
	serverError    = "Error"
)

type Server struct {
	isRunning int32
	report    *perf.Report
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) runHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if atomic.LoadInt32(&s.isRunning) > 0 {
		fmt.Fprint(w, serverRunning)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestSize))
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		fmt.Printf("Error reading request body: %v\n", err)
		return
	}
	defer r.Body.Close()

	runConf, err := perf.ReadRunConfByByte(body)
	if err != nil {
		http.Error(w, "Invalid configuration", http.StatusBadRequest)
		fmt.Printf("ReadRunConfByByte error: %v\n", err)
		return
	}

	atomic.StoreInt32(&s.isRunning, 1)
	go func() {
		defer atomic.StoreInt32(&s.isRunning, 0)
		s.report = runConf.Run()
	}()

	fmt.Fprint(w, serverStarted)
	fmt.Printf("Running configuration:\n%s\n", body)
}

func (s *Server) reportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if atomic.LoadInt32(&s.isRunning) == 1 {
		fmt.Fprint(w, serverRunning)
		return
	}

	if s.report == nil {
		http.Error(w, "No report available", http.StatusNotFound)
		return
	}

	yamlData, err := yaml.Marshal(s.report)
	if err != nil {
		http.Error(w, serverError, http.StatusInternalServerError)
		fmt.Printf("Error marshaling report: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(yamlData)
}

func Start(port string) error {
	server := NewServer()

	mux := http.NewServeMux()
	mux.HandleFunc("/run", server.runHandler)
	mux.HandleFunc("/report", server.reportHandler)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	fmt.Printf("Server starting on port %s\n", port)
	return srv.ListenAndServe()
}
