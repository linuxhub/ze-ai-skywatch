package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
	"github.com/linuxhub/ze-ai-skywatch/internal/netconfig"
)

func fixtureRouteTable() netconfig.RouteTable {
	return netconfig.RouteTable{At: time.Now(), Rows: []netconfig.RouteRule{
		{Destination: "192.168.110.0/24", Gateway: "10.6.0.13", Interface: "utun4", Tunnel: true},
		{Destination: "192.168.11.0/24", Gateway: "10.6.0.13", Interface: "utun4", Tunnel: true},
		{Destination: "172.16.10.0/24", Gateway: "10.6.0.13", Interface: "utun4", Tunnel: true},
		{Destination: "0.0.0.0/0", Gateway: "172.18.13.236", Interface: "en8"},
		{Destination: "::/0", Gateway: "fe80::%utun0", Interface: "utun0", Scoped: true, Tunnel: true},
	}}
}

func TestRoutingTabQueryCopyAndAdapterVisibility(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page = "netconfig"
	a.netconfig.snapshot = fixtureNetconfig()
	a.netconfig.snapshot.Routing = fixtureRouteTable()
	a.netconfig.snapshot.Adapters = append(a.netconfig.snapshot.Adapters, netconfig.Adapter{Name: "utun4", Service: "utun4", Kind: "隧道／VPN 接口", Virtual: true, Up: true, Status: "已连接", Addresses: []netconfig.Address{{IP: "10.6.0.14", Prefix: 32}}})
	visible := false
	for _, adapter := range a.netconfigAdapters() {
		visible = visible || adapter.Name == "utun4"
	}
	if !visible {
		t.Fatal("VPN interface with a forwarding route must be visible by default")
	}
	tester := ui.NewTester(a.view, 1360, 850)
	tester.SetLocale("zh-Hans")
	if err := tester.Click("路由与分流"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("192.168.110.0/24") || !tester.HasText("10.6.0.13") || !tester.HasText("utun4") || tester.HasText("::/0") {
		t.Fatal("VPN filter includes scoped system routes or omitted VPN routes")
	}
	if err := tester.Click("查询目标 IP 路由"); err != nil {
		t.Fatal(err)
	}
	tester.Type("192.168.110.20")
	if !tester.HasText("192.168.110.0/24 → 10.6.0.13 → utun4（VPN／隧道）") {
		t.Fatal("target route lookup missing", tester.Texts())
	}
	if err := tester.Click("复制出口推断"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tester.Clipboard(), "utun4") {
		t.Fatal(tester.Clipboard())
	}
	if err := tester.Click("复制全部路由"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tester.Clipboard(), "0.0.0.0/0") || !strings.Contains(tester.Clipboard(), "仅限接口") {
		t.Fatal("copy omitted routes outside current filter")
	}
	if err := tester.Click("网卡与代理"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("网卡列表") {
		t.Fatal("adapter tab did not return")
	}
	if err := tester.Click("路由与分流"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if tester.FramePending() {
		t.Fatal("route view did not settle after switching tabs")
	}
}

func TestConnectionRoutesSearchDetailsAndStaleData(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page = "connections"
	a.netconfig.snapshot.Routing = fixtureRouteTable()
	row := connections.Connection{PID: 20, Started: 1000, Process: "InternalApp", Protocol: "TCP", Family: "IPv4", State: "ESTABLISHED", Local: connections.Endpoint{IP: "10.6.0.14", Port: "50000"}, Remote: connections.Endpoint{IP: "192.168.110.20", Port: "443"}, Interface: "utun4"}
	a.connections.snapshot = connections.Snapshot{At: time.Now(), Rows: []connections.Connection{row}}
	a.connections.query = "10.6.0.13"
	if len(a.filteredConnections()) != 1 {
		t.Fatal("cannot search route next-hop")
	}
	tester := ui.NewTester(a.view, 1800, 1000)
	tester.SetLocale("zh-Hans")
	a.connections.detail, a.connections.detailOpen = row, true
	tester.Frame()
	if err := tester.Click("路由分析"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("路由出口推断") || !tester.HasText("VPN／隧道 · utun4") || !tester.HasText("192.168.110.0/24") {
		t.Fatal("connection route details missing")
	}
	if err := tester.Click("复制端点信息"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tester.Clipboard(), "路由推断：192.168.110.0/24 → 10.6.0.13 → utun4") {
		t.Fatal(tester.Clipboard())
	}
	snapshot := fixtureNetconfig()
	snapshot.Routing = netconfig.RouteTable{Err: "路由读取失败"}
	a.publishNetconfig(a.netconfig.generation, snapshot)
	if a.matchRoute(row.Remote.IP).Found || len(a.netconfig.snapshot.Routing.Rows) == 0 || a.netconfig.snapshot.Routing.Err == "" {
		t.Fatal("stale rules were used or the old table discarded")
	}
	if len(a.filteredConnections()) != 0 {
		t.Fatal("route search used stale projection")
	}
}

func TestRouteLayouts(t *testing.T) {
	for _, width := range []int{1180, 1360, 1880} {
		for _, dark := range []bool{false, true} {
			a := fixtureApp()
			a.page, a.netconfig.tab = "netconfig", 1
			a.netconfig.snapshot = fixtureNetconfig()
			a.netconfig.snapshot.Routing = fixtureRouteTable()
			for i := 0; i < 65; i++ {
				a.netconfig.snapshot.Routing.Rows = append(a.netconfig.snapshot.Routing.Rows, netconfig.RouteRule{Destination: fmt.Sprintf("10.20.%d.0/24", i), Gateway: "172.18.13.236", Interface: "en8"})
			}
			a.netconfig.routeFilter = "全部路由"
			a.netconfig.routeQuery = "192.168.110.20"
			height := 850
			if width == 1180 {
				height = 700
			} else if width == 1880 {
				height = 1240
			}
			tester := ui.NewTester(a.view, width, height)
			tester.SetLocale("zh-Hans")
			tester.SetDark(dark)
			tester.Frame()
			if tester.FramePending() {
				t.Fatal("route layout did not settle")
			}
			if directory := os.Getenv("MAC_MONITOR_SCREENSHOTS"); directory != "" {
				if err := os.MkdirAll(directory, 0755); err != nil {
					t.Fatal(err)
				}
				name := "routes-" + map[bool]string{false: "light", true: "dark"}[dark]
				if width == 1180 {
					name += "-compact"
				} else if width == 1880 {
					name += "-tall"
				}
				file, err := os.Create(filepath.Join(directory, name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, tester.Image())
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			a.cancel()
		}
	}
}

func TestRoutingTableFillsAvailableHeight(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page, a.netconfig.tab = "netconfig", 1
	a.netconfig.snapshot = fixtureNetconfig()
	a.netconfig.snapshot.Routing = fixtureRouteTable()
	a.netconfig.routeFilter = "全部路由"
	for i := 0; i < 80; i++ {
		a.netconfig.snapshot.Routing.Rows = append(a.netconfig.snapshot.Routing.Rows, netconfig.RouteRule{Destination: fmt.Sprintf("10.20.%d.0/24", i), Gateway: "172.18.13.236", Interface: "en8"})
	}
	tester := ui.NewTester(a.view, 1360, 850)
	tester.SetLocale("zh-Hans")
	find := func(label string) ui.Rect {
		t.Helper()
		r, ok := tester.Find(label)
		if !ok {
			t.Fatalf("missing %s", label)
		}
		return r
	}
	initial := find("系统路由与分流列表")
	tester.SetSize(1880, 1240)
	tall := find("系统路由与分流列表")
	if tall.H < initial.H+380 {
		t.Fatalf("table did not use added window height: %+v -> %+v", initial, tall)
	}
	for _, size := range [][2]int{{1880, 1240}, {1180, 700}, {1360, 850}} {
		tester.SetSize(size[0], size[1])
		a.netconfig.routeQuery = "192.168.110.20"
		tester.Frame()
		card := find("路由与 VPN 分流卡片")
		if bottom := card.Y + card.H; bottom < float32(size[1]-60) || bottom > float32(size[1]-45) {
			t.Fatalf("card left unused space or overflowed at %v: %+v", size, card)
		}
		table := find("系统路由与分流列表")
		query := find("查询目标 IP 路由")
		if table.H < 70 || query.Y < table.Y+table.H || query.Y+query.H > card.Y+card.H-20 {
			t.Fatalf("table or query clipped at %v: table %+v; query %+v; card %+v", size, table, query, card)
		}
		tester.Scroll(table.X+table.W/2, table.Y+table.H/2, 0, 10000)
		tester.Frame()
		if !tester.HasText("10.20.79.0/24") {
			t.Fatalf("cannot reach last route at %v", size)
		}
		if err := tester.Click("复制出口推断"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(tester.Clipboard(), "utun4") {
			t.Fatal("route lookup is not accessible after resizing")
		}
	}
}
