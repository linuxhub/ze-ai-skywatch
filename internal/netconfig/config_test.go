package netconfig

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestAddressMaskAndProxyRedaction(t *testing.T) {
	address := Address{IP: "192.168.1.8", Prefix: 23}
	if address.String() != "192.168.1.8/23 · 掩码 255.255.254.0" {
		t.Fatal(address.String())
	}
	if strings.Contains((Address{IP: "fe80::1", Prefix: 64}).String(), "掩码") {
		t.Fatal("IPv6 was given an IPv4 mask")
	}
	proxy := Proxy{Kind: "PAC", Known: true, Enabled: true, URL: "https://user:password@example.com/config.pac?token=secret#key"}
	snapshot := Snapshot{Proxies: []Proxy{proxy}}
	text := snapshot.Text()
	for _, secret := range []string{"user", "password", "secret", "token", "#key"} {
		if strings.Contains(text, secret) {
			t.Fatalf("copied proxy leaked %s: %s", secret, text)
		}
	}
	if !strings.Contains(text, "https://example.com/config.pac") {
		t.Fatal(text)
	}
}

func TestProxyStatusDoesNotInferDisabledFromMissingData(t *testing.T) {
	if (Snapshot{}).ProxyStatus() != Unknown {
		t.Fatal("missing data reported disabled")
	}
	snapshot := Snapshot{Proxies: []Proxy{{Kind: "HTTP"}, {Kind: "SOCKS", Known: true, Enabled: true}}}
	if snapshot.ProxyStatus() != "已启用" {
		t.Fatal("unknown proxy hid a known enabled proxy")
	}
	snapshot.Proxies[1].Enabled = false
	if snapshot.ProxyStatus() != Unknown {
		t.Fatal("partial data reported disabled")
	}
}

func TestCheckProxyOnlyDialsEnabledEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	proxy := Proxy{Kind: "HTTP", Known: true, Enabled: true, Server: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port}
	if result := CheckProxy(context.Background(), proxy); !strings.HasPrefix(result, "TCP 可连接") {
		t.Fatal(result)
	}
	proxy.Enabled = false
	if result := CheckProxy(context.Background(), proxy); result != "无可检测的固定代理端点" {
		t.Fatal(result)
	}
	proxy.Enabled, proxy.Port = true, 65536
	if result := CheckProxy(context.Background(), proxy); result != "无可检测的固定代理端点" {
		t.Fatal(result)
	}
}
