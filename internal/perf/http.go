package perf

import (
	"bytes"
	"net/http"
	"strings"
)

type HTTPconf struct {
	Name      string            `yaml:"Name"`
	Proto     string            `yaml:"Proto"`
	Method    string            `yaml:"Method"`
	URI       string            `yaml:"URI"`
	Header    map[string]string `yaml:"Header"`
	Body      string            `yaml:"Body"`
	UseParams []string          `yaml:"UseParams"`
	reqBytes  []byte
	paramsMap map[string]Params
}

func (h *HTTPconf) SetReqBytes() error {
	h.reqBytes = nil
	//get req bytes
	var reqBytes []byte
	body := strings.NewReader(h.Body)
	req, err := http.NewRequest(h.Method, h.URI, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mmin")
	for k, v := range h.Header {
		req.Header.Set(k, v)
	}
	var buf bytes.Buffer
	err = req.Write(&buf)
	if err != nil {
		return err
	}
	reqBytes = buf.Bytes()
	if h.Proto == "HTTP/1.0" {
		reqBytes = []byte(strings.Replace(string(buf.Bytes()), "HTTP/1.1", "HTTP/1.0", 1))
	}
	//set
	h.reqBytes = reqBytes
	return nil
}

func (h *HTTPconf) GetReqBytes() []byte {
	if len(h.UseParams) == 0 {
		return h.reqBytes
	}
	var newReqBytes []byte = h.reqBytes
	for _, paramName := range h.UseParams {
		if param, yes := h.paramsMap[paramName]; yes {
			newReqBytes = param.replace(newReqBytes)
		}
	}
	return newReqBytes
}
