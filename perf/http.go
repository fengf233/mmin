package perf

import (
	"bytes"
	"net/http"
	"strings"
)

type HTTPconf struct {
	Name   string            `yaml:"Name"`
	Proto  string            `yaml:"Proto"`
	Method string            `yaml:"Method"`
	URI    string            `yaml:"URI"`
	Header map[string]string `yaml:"Header"`
	Body   string            `yaml:"Body"`
}

func (h *HTTPconf) GetReqBytes() ([]byte, error) {
	var reqBytes []byte
	body := strings.NewReader(h.Body)
	req, err := http.NewRequest(h.Method, h.URI, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "goperf")
	for k, v := range h.Header {
		req.Header.Set(k, v)
	}
	var buf bytes.Buffer
	err = req.Write(&buf)
	if err != nil {
		return nil, err
	}
	reqBytes = buf.Bytes()
	if h.Proto == "HTTP/1.0" {
		reqBytes = []byte(strings.Replace(string(buf.Bytes()), "HTTP/1.1", "HTTP/1.0", 1))
	}
	return reqBytes, nil
}
