package perf

import (
	"net"
	"net/url"
	"strings"
)

func UrlIsHttps(urlstr string) bool {
	if strings.Contains(urlstr, "https") {
		return true
	} else {
		return false
	}
}

func GetDstByUrl(urlstr string) (string, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return "", err
	}

	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}
	ipstr := ips[0].String()
	var dst string
	if strings.Contains(ipstr, ":") {
		dst = "[" + ipstr + "]" + ":" + port
	} else {
		dst = ipstr + ":" + port
	}
	return dst, nil

}
