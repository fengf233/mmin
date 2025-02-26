package perf

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	defaultHTTPPort  = "80"
	defaultHTTPSPort = "443"
)

// UrlIsHttps 检查URL是否为HTTPS
func UrlIsHttps(urlstr string) bool {
	return strings.Contains(urlstr, "https")
}

// GetDstByUrl 从URL获取目标地址
func GetDstByUrl(urlstr string) (string, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return "", fmt.Errorf("解析URL失败: %w", err)
	}

	// 获取端口号
	port := u.Port()
	if port == "" {
		port = defaultHTTPPort
		if u.Scheme == "https" {
			port = defaultHTTPSPort
		}
	}

	// 解析主机名
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("解析主机名失败: %w", err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("未找到IP地址: %s", host)
	}

	// 构建目标地址
	ipstr := ips[0].String()
	if strings.Contains(ipstr, ":") {
		return fmt.Sprintf("[%s]:%s", ipstr, port), nil
	}
	return fmt.Sprintf("%s:%s", ipstr, port), nil
}
