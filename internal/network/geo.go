package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"time"
)

type Location struct {
	IP, Country, City, Organization string
	Latitude, Longitude             float64
	ASN                             int
	Known                           bool
	Err                             string
	Limited                         bool
}

func (l Location) Label() string {
	if !l.Known {
		return "位置未知"
	}
	if l.City != "" {
		return l.Country + " · " + l.City
	}
	return l.Country
}
func PublicIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// Exclude CGNAT, benchmarking, documentation and other special ranges. The
	// enrichment service should only receive publicly routable endpoint IPs.
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

// Lookup is called only after the explicit online lookup switch is enabled.
// It sends a destination IP, never PID, process name or local address. No DNS
// reverse lookup or URL inference is performed automatically.
func Lookup(ctx context.Context, client *http.Client, ip string) Location {
	l := Location{IP: ip}
	if !PublicIP(ip) {
		l.Err = "非公网 IP 不查询"
		return l
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/"+ip, nil)
	if err != nil {
		l.Err = err.Error()
		return l
	}
	resp, err := client.Do(req)
	if err != nil {
		l.Err = "位置查询失败"
		return l
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		l.Limited = resp.StatusCode == http.StatusTooManyRequests
		l.Err = fmt.Sprintf("位置服务返回 HTTP %d", resp.StatusCode)
		return l
	}
	var body struct {
		IP         string  `json:"ip"`
		Success    bool    `json:"success"`
		Country    string  `json:"country"`
		City       string  `json:"city"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		Connection struct {
			ASN int    `json:"asn"`
			Org string `json:"org"`
			ISP string `json:"isp"`
		} `json:"connection"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&body); err != nil || !body.Success || net.ParseIP(body.IP) == nil || !net.ParseIP(body.IP).Equal(net.ParseIP(ip)) {
		l.Err = "位置服务未提供有效结果"
		return l
	}
	if math.IsNaN(body.Latitude) || math.IsNaN(body.Longitude) || math.Abs(body.Latitude) > 90 || math.Abs(body.Longitude) > 180 || strings.TrimSpace(body.Country) == "" {
		l.Err = "位置坐标无效"
		return l
	}
	l.Country = body.Country
	l.City = body.City
	l.Latitude = body.Latitude
	l.Longitude = body.Longitude
	l.Organization = body.Connection.Org
	if l.Organization == "" {
		l.Organization = body.Connection.ISP
	}
	l.ASN = body.Connection.ASN
	l.Known = true
	return l
}
