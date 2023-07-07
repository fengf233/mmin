package server

import (
	"fmt"
	"io/ioutil"
	"mmin/perf"
	"net/http"
	"sync/atomic"

	"gopkg.in/yaml.v2"
)

var (
	isRunning int32
	report    *perf.Report
)

func runHandler(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&isRunning) > 0 {
		fmt.Fprint(w, "running")
		return
	}
	atomic.AddInt32(&isRunning, 1)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprint(w, "Error reading request body")
		fmt.Println("Error reading request body", err.Error())
		return
	}
	runConf, err := perf.ReadRunConfByByte(body)
	if err != nil {
		fmt.Fprint(w, "ReadRunConfByByte")
		fmt.Println("ReadRunConfByByte:", err.Error())
		return
	}
	go func() {
		report = runConf.Run()
		atomic.AddInt32(&isRunning, -1)
	}()
	fmt.Fprint(w, "start")
	fmt.Println("running conf:\n", string(body))
	return
}

func reportHandler(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&isRunning) == 1 {
		fmt.Fprint(w, "Running")
		return
	}
	yamlData, err := yaml.Marshal(report)
	if err != nil {
		fmt.Fprint(w, "Error")
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Write(yamlData)
}

func Start(port string) {
	isRunning = 0

	http.HandleFunc("/run", runHandler)
	http.HandleFunc("/report", reportHandler)

	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("HTTP server failed to start:", err)
	}
}
