package netconfig

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRouteMatcherLongestPrefixAndMetric(t *testing.T) {
	table := RouteTable{At: time.Now(), Rows: []RouteRule{
		{Destination: "0.0.0.0/0", Interface: "en8", Gateway: "172.18.13.236"},
		{Destination: "192.168.110.0/24", Interface: "utun4", Gateway: "10.6.0.13", Tunnel: true, Metric: 500},
		{Destination: "192.168.110.8/32", Interface: "en0", Gateway: "192.168.50.1", Metric: 20},
		{Destination: "192.168.110.8/32", Interface: "en8", Gateway: "172.18.13.236", Metric: 30},
		{Destination: "192.168.110.9/32", Interface: "utun4", Reject: true},
	}}
	matcher := NewRouteMatcher(table)
	for _, test := range []struct{ destination, iface, prefix, path string }{
		{"192.168.110.20", "utun4", "192.168.110.0/24", "VPN／隧道"},
		{"192.168.110.8", "en0", "192.168.110.8/32", "普通接口"},
		{"1.1.1.1", "en8", "0.0.0.0/0", "普通接口"},
		{"192.168.110.9", "utun4", "192.168.110.9/32", "拒绝／丢弃"},
		{"::ffff:192.168.110.20", "utun4", "192.168.110.0/24", "VPN／隧道"},
	} {
		match := matcher.Match(test.destination)
		if !match.Found || match.Rule.Interface != test.iface || match.Rule.Destination != test.prefix || match.Rule.Path() != test.path {
			t.Fatalf("%s: %+v", test.destination, match)
		}
	}
	if match := matcher.Match(""); match.Found || match.Reason != "无远端地址" {
		t.Fatal(match)
	}
	if match := matcher.Match("example.com"); match.Found {
		t.Fatal("lookup must not resolve domains")
	}
}

func TestRouteMatcherFullTunnelSplitDefaults(t *testing.T) {
	matcher := NewRouteMatcher(RouteTable{At: time.Now(), Rows: []RouteRule{
		{Destination: "0.0.0.0/0", Interface: "en8"},
		{Destination: "0.0.0.0/1", Interface: "utun4", Tunnel: true},
		{Destination: "128.0.0.0/1", Interface: "utun4", Tunnel: true},
		{Destination: "::/0", Interface: "en0"},
		{Destination: "::/1", Interface: "utun5", Tunnel: true},
		{Destination: "8000::/1", Interface: "utun5", Tunnel: true},
	}})
	for _, destination := range []string{"1.1.1.1", "200.1.1.1", "2606:4700::1111", "a001::1"} {
		if match := matcher.Match(destination); !match.Found || !match.Rule.Tunnel {
			t.Fatal(destination, match)
		}
	}
}

func TestRouteMatcherScopedAmbiguousAndUnavailable(t *testing.T) {
	table := RouteTable{At: time.Now(), Rows: []RouteRule{
		{Destination: "0.0.0.0/0", Interface: "en8", Metric: 10},
		{Destination: "0.0.0.0/0", Interface: "en0", Metric: 10},
		{Destination: "::/0", Interface: "utun0", Tunnel: true, Scoped: true},
		{Destination: "fe80::/64", Interface: "en0", Scoped: true},
	}}
	matcher := NewRouteMatcher(table)
	if match := matcher.Match("1.1.1.1"); match.Found || !strings.Contains(match.Reason, "多个出口") {
		t.Fatal(match)
	}
	if match := matcher.Match("2606:4700::1111"); match.Found {
		t.Fatal("scoped VPN route was treated as a global default")
	}
	if match := matcher.Match("fe80::1"); match.Found || !strings.Contains(match.Reason, "%接口") {
		t.Fatal(match)
	}
	if match := matcher.Match("fe80::1%en0"); !match.Found || match.Rule.Interface != "en0" {
		t.Fatal(match)
	}
	table.Err = "采集失败"
	if match := NewRouteMatcher(table).Match("fe80::1%en0"); match.Found {
		t.Fatal("stale routing data was used")
	}
	table.Err, table.Truncated = "", true
	if match := NewRouteMatcher(table).Match("fe80::1%en0"); match.Found {
		t.Fatal("truncated routing data was used")
	}
	if table.Rows[2].TunnelPolicy() {
		t.Fatal("scoped default was counted as VPN diversion")
	}
}

func TestRouteLimitDeduplicationAndText(t *testing.T) {
	table := RouteTable{At: time.Now(), Rows: []RouteRule{
		{Destination: "192.168.110.8/24", Interface: "utun4", Gateway: "10.6.0.13", Tunnel: true},
		{Destination: "192.168.110.0/24", Interface: "utun4", Gateway: "10.6.0.13", Tunnel: true},
	}}
	finishRoutes(&table)
	if len(table.Rows) != 1 || table.Rows[0].Destination != "192.168.110.0/24" {
		t.Fatal(table)
	}
	if !strings.Contains(table.Text(), "10.6.0.13\tutun4\tVPN／隧道") {
		t.Fatal(table.Text())
	}
	for index := 0; index <= MaxRoutes; index++ {
		table.Rows = append(table.Rows, RouteRule{Destination: "0.0.0.0/0", Interface: fmt.Sprintf("interface-%d", index)})
	}
	finishRoutes(&table)
	if !table.Truncated || len(table.Rows) != MaxRoutes {
		t.Fatal("route limit not enforced")
	}
}
