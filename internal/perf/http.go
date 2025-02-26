package perf

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type HTTPconf struct {
	Name       string            `yaml:"Name"`
	Proto      string            `yaml:"Proto"`
	Method     string            `yaml:"Method"`
	URI        string            `yaml:"URI"`
	Header     map[string]string `yaml:"Header"`
	Body       string            `yaml:"Body"`
	UseParams  []string          `yaml:"UseParams"`
	FileUpload string            `yaml:"FileUpload"`
	reqBytes   []byte
	paramsMap  map[string]Params
}

func (h *HTTPconf) SetReqBytes() error {
	h.reqBytes = nil
	//get req bytes
	var reqBytes []byte
	var body *strings.Reader

	if h.FileUpload != "" {
		// Create multipart form data for file upload
		buf := &bytes.Buffer{}
		writer := multipart.NewWriter(buf)
		file, err := os.Open(h.FileUpload)
		if err != nil {
			return err
		}
		defer file.Close()

		part, err := writer.CreateFormFile("file", filepath.Base(h.FileUpload))
		if err != nil {
			return err
		}
		_, err = io.Copy(part, file)
		if err != nil {
			return err
		}
		writer.Close()

		// Set content type header
		if h.Header == nil {
			h.Header = make(map[string]string)
		}
		h.Header["Content-Type"] = writer.FormDataContentType()

		body = strings.NewReader(buf.String())
	} else {
		body = strings.NewReader(h.Body)
	}

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
