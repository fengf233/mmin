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

// HTTPconf represents HTTP request configuration
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

const (
	defaultUserAgent = "mmin"
	fileFormKey      = "file"
)

func (h *HTTPconf) SetReqBytes() error {
	h.reqBytes = nil

	// 使用 bytes.Buffer 来存储请求体
	buf := &bytes.Buffer{}

	if h.FileUpload != "" {
		if err := h.handleFileUpload(buf); err != nil {
			return err
		}
	} else {
		buf.WriteString(h.Body)
	}

	// 创建请求
	req, err := http.NewRequest(h.Method, h.URI, buf)
	if err != nil {
		return err
	}

	// 设置请求头
	req.Header.Set("User-Agent", defaultUserAgent)
	for k, v := range h.Header {
		req.Header.Set(k, v)
	}

	// 将请求写入缓冲区
	var reqBuf bytes.Buffer
	if err := req.Write(&reqBuf); err != nil {
		return err
	}

	// 处理协议版本
	if h.Proto == "HTTP/1.0" {
		h.reqBytes = []byte(strings.Replace(reqBuf.String(), "HTTP/1.1", "HTTP/1.0", 1))
	} else {
		h.reqBytes = reqBuf.Bytes()
	}

	return nil
}

// handleFileUpload 处理文件上传逻辑
func (h *HTTPconf) handleFileUpload(buf *bytes.Buffer) error {
	writer := multipart.NewWriter(buf)
	defer writer.Close()

	file, err := os.Open(h.FileUpload)
	if err != nil {
		return err
	}
	defer file.Close()

	part, err := writer.CreateFormFile(fileFormKey, filepath.Base(h.FileUpload))
	if err != nil {
		return err
	}

	if _, err := io.Copy(part, file); err != nil {
		return err
	}

	// 设置 Content-Type header
	if h.Header == nil {
		h.Header = make(map[string]string)
	}
	h.Header["Content-Type"] = writer.FormDataContentType()

	return nil
}

func (h *HTTPconf) GetReqBytes() []byte {
	if len(h.UseParams) == 0 {
		return h.reqBytes
	}

	newReqBytes := h.reqBytes
	for _, paramName := range h.UseParams {
		if param, exists := h.paramsMap[paramName]; exists {
			newReqBytes = param.replace(newReqBytes)
		}
	}
	return newReqBytes
}
