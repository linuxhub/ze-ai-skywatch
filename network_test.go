package main

import (
	"bytes"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/network"
)

func fixtureNetwork(a *app) {
	a.config.NetworkGeo = true // Fixture coordinates; headless tests do not query.
	n := &a.network
	n.enabled = true
	n.started = time.Now().Add(-time.Minute)
	n.light = network.LightSnapshot{At: time.Now(), Download: 12.4 * 1024 * 1024, Upload: 1.8 * 1024 * 1024, Received: 880 << 20, Sent: 72 << 20, Ready: true, Interfaces: []string{"en0"}}
	n.snapshot.At = time.Now()
	names := []string{"Google Chrome", "node", "WeChat", "Code Helper", "Dropbox", "cloudd", "Discord"}
	ips := []string{"142.250.190.14", "1.1.1.1", "223.5.5.5", "17.253.144.10", "8.8.8.8", "127.0.0.1", "[2001:4860:4860::8888]"}
	for i, name := range names {
		id := network.Identity{PID: int32(200 + i), Started: 1000}
		traffic := network.Traffic{Download: float64(7-i) * 170000, Upload: float64(i+1) * 13000, Received: uint64(7-i) * 90 << 20, Sent: uint64(i+1) * 10 << 20, Ready: true}
		n.snapshot.Processes = append(n.snapshot.Processes, network.Process{Identity: id, Name: name, App: name, Group: name, Traffic: traffic, Connections: 1, Active: true})
		n.snapshot.Connections = append(n.snapshot.Connections, network.Connection{Identity: id, Process: name, Protocol: "TCP", Local: fmt.Sprintf("192.168.1.10:%d", 54000+i), Remote: ips[i] + ":443", Interface: "en0", State: "Established", Traffic: traffic})
	}
	n.geo["142.250.190.14"] = network.Location{IP: "142.250.190.14", Country: "United States", City: "Mountain View", Organization: "Google LLC", Latitude: 37.4, Longitude: -122.1, ASN: 15169, Known: true}
	n.geo["223.5.5.5"] = network.Location{IP: "223.5.5.5", Country: "China", City: "Hangzhou", Organization: "Alibaba", Latitude: 30.3, Longitude: 120.2, ASN: 37963, Known: true}
	n.geo["1.1.1.1"] = network.Location{IP: "1.1.1.1", Country: "Australia", City: "Sydney", Organization: "Cloudflare", Latitude: -33.8, Longitude: 151.2, ASN: 13335, Known: true}
	for i := 0; i < 60; i++ {
		n.history = append(n.history, networkPoint{At: time.Now().Add(time.Duration(i-60) * 2 * time.Second), Download: float64(9+i%7) * 90000, Upload: float64(1+i%3) * 50000})
	}
}
func TestNetworkSwitchesAndFiltering(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page = "network"
	tt := ui.NewTester(a.view, 1360, 850)
	tt.SetLocale("zh-Hans")
	if a.network.enabled || a.config.NetworkAnimation || a.config.NetworkGeo {
		t.Fatal("network must default to lightweight/off")
	}
	if err := tt.Click("开始分析"); err != nil {
		t.Fatal(err)
	}
	if !a.network.enabled {
		t.Fatal("analysis did not start")
	}
	fixtureNetwork(a)
	tt.Frame()
	if err := tt.Click("Google Chrome"); err != nil {
		t.Fatal(err)
	}
	if a.network.filter != "Google Chrome" || len(a.networkConnections()) != 1 {
		t.Fatalf("selection filter: %q %+v", a.network.filter, a.networkConnections())
	}
	if err := tt.Click("显示全部进程"); err != nil {
		t.Fatal(err)
	}
	if len(a.networkConnections()) != 7 {
		t.Fatal("failed to clear process filter")
	}
	if err := tt.Click("搜索网络连接"); err != nil {
		t.Fatal(err)
	}
	tt.Type("223.5.5.5")
	if rows := a.networkConnections(); len(rows) != 1 || rows[0].Process != "WeChat" {
		t.Fatalf("IP filter: %+v", rows)
	}
	if err := tt.Click("停止分析"); err != nil {
		t.Fatal(err)
	}
	if a.network.enabled || len(a.network.snapshot.Connections) != 0 || len(a.network.geo) != 0 {
		t.Fatal("stop retained detailed data")
	}
	if !tt.HasText("轻量监控正在运行") {
		t.Fatal("lightweight mode not restored")
	}
	// The same switch is reachable from settings, with stable identity across
	// disappearing and returning map/table controls.
	a.page = "settings"
	tt.Frame()
	if err := tt.Click("启用详细网络分析（本次运行）"); err != nil {
		t.Fatal(err)
	}
	if !a.network.enabled {
		t.Fatal("settings switch did not enable")
	}
	if err := tt.Click("启用详细网络分析（本次运行）"); err != nil {
		t.Fatal(err)
	}
	if a.network.enabled {
		t.Fatal("settings switch did not disable")
	}
}

func TestNetworkRenderLayouts(t *testing.T) {
	for _, size := range [][2]int{{1180, 700}, {1360, 850}, {1360, 900}, {1900, 1200}, {2560, 1380}} {
		for _, dark := range []bool{false, true} {
			a := fixtureApp()
			a.page = "network"
			fixtureNetwork(a)
			tt := ui.NewTester(a.view, size[0], size[1])
			tt.SetLocale("zh-Hans")
			tt.SetDark(dark)
			for _, mode := range []string{"analysis", "detail", "lightweight"} {
				switch mode {
				case "detail":
					a.network.detail = a.network.snapshot.Connections[0]
					a.network.detailOpen = true
				case "lightweight":
					a.setNetworkAnalysis(false)
				}
				tt.Frame()
				if tt.FramePending() {
					t.Fatalf("static network view did not settle: %s", mode)
				}
				if dir := os.Getenv("MAC_MONITOR_SCREENSHOTS"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					file, err := os.Create(filepath.Join(dir, fmt.Sprintf("network-%s-%dx%d-%t.png", mode, size[0], size[1], dark)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tt.Image())
					_ = file.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a.cancel()
		}
	}
}

func TestNetworkPanelsFollowWindowHeight(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page = "network"
	fixtureNetwork(a)
	tt := ui.NewTester(a.view, 1360, 1000)
	tt.SetLocale("zh-Hans")
	find := func(label string) ui.Rect {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("missing %s", label)
		}
		return r
	}
	analysis := find("网络流量分析区")
	connections := find("网络连接列表")
	tt.SetSize(1900, 1300)
	largeAnalysis := find("网络流量分析区")
	largeConnections := find("网络连接列表")
	if largeAnalysis.H < analysis.H+190 || largeConnections.H < connections.H+90 {
		t.Fatalf("panels did not share added height: analysis %+v -> %+v; connections %+v -> %+v", analysis, largeAnalysis, connections, largeConnections)
	}
	if largeConnections.Y+largeConnections.H > 1220 {
		t.Fatal("connections/footer extend beyond the window")
	}
	tt.SetSize(1180, 700)
	if r := find("网络流量分析区"); r.H < 358 {
		t.Fatalf("compact window crushed analysis: %+v", r)
	}
	// Both table scrolling and the page's overflow must remain reachable.
	r := find("网络流量分析区")
	tt.Scroll(r.X+r.W-10, r.Y+r.H/2, 0, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	r = find("网络连接列表")
	if r.Y+r.H > 670 || r.Y < 56 {
		t.Fatalf("compact page cannot scroll to connections: %+v", r)
	}
	tt.SetSize(1900, 1200)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if r := find("网络流量分析区"); r.Y < 200 || tt.FramePending() {
		t.Fatalf("resize retained compact scroll offset or failed to settle: %+v", r)
	}
}

func TestNetworkMapKeepsProportionsAndHitTargets(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page = "network"
	fixtureNetwork(a)
	tt := ui.NewTester(a.view, 1360, 850)
	tt.SetLocale("zh-Hans")
	for _, size := range [][2]int{{1360, 850}, {2560, 1380}, {1180, 700}, {1900, 1200}} {
		tt.SetSize(size[0], size[1])
		layer, ok := tt.Find("连接去向地图")
		if !ok || math.Abs(float64(layer.W/layer.H-2.4)) > 0.01 {
			t.Fatalf("map distorted at %v: %+v", size, layer)
		}
		for _, d := range a.mapDestinations() {
			if d.IP != "223.5.5.5" {
				continue
			}
			x, y := layer.X+layer.W*d.X, layer.Y+layer.H*d.Y
			tt.ClickAt(x, y)
			if !a.network.detailOpen || a.network.detail.RemoteIP() != d.IP {
				t.Fatalf("map hit target misaligned at %v", size)
			}
			if err := tt.Click("关闭连接详情"); err != nil {
				t.Fatal(err)
			}
		}
		if tt.FramePending() {
			t.Fatalf("map layout failed to settle at %v", size)
		}
	}
}

func TestNetworkAggregatesAndMapLimit(t *testing.T) {
	a := newApp()
	defer a.cancel()
	fixtureNetwork(a)
	p := a.network.snapshot.Processes[0]
	p.PID = 999
	p.Name = "Chrome Helper"
	a.network.snapshot.Processes = append(a.network.snapshot.Processes, p)
	rows := a.networkProcesses()
	if len(rows) != 7 || len(rows[0].Processes) != 2 {
		t.Fatalf("helper not grouped: %+v", rows)
	}
	a.network.group = false
	if len(a.networkProcesses()) != 8 {
		t.Fatal("process mode must separate PIDs")
	}
	for i := 0; i < 50; i++ {
		a.network.snapshot.Connections = append(a.network.snapshot.Connections, network.Connection{Identity: p.Identity, Remote: fmt.Sprintf("11.0.0.%d:443", i+1), Local: "192.168.1.1:5", Protocol: "TCP"})
	}
	a.network.snapshot.At = time.Now()
	if len(a.mapDestinations()) > 20 {
		t.Fatal("map should bound rendered endpoints")
	}
}

func TestNetworkAnimationOnlyRepaintsAndPausesWhenHidden(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page = "network"
	fixtureNetwork(a)
	a.config.NetworkAnimation = true
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; a.view(c) }, 1360, 850)
	before := append([]byte(nil), tt.Image().Pix...)
	count := builds
	tt.Advance(200 * time.Millisecond)
	if builds != count {
		t.Fatal("animation rebuilt the UI instead of repainting")
	}
	if bytes.Equal(before, tt.Image().Pix) {
		t.Fatal("animation did not move")
	}
	a.network.visible = false
	tt.Frame()
	before = append(before[:0], tt.Image().Pix...)
	count = builds
	tt.Advance(time.Second)
	if builds != count || !bytes.Equal(before, tt.Image().Pix) {
		t.Fatal("hidden window continued animating")
	}
}

func BenchmarkNetworkStaticProjection(b *testing.B) {
	a := newApp()
	defer a.cancel()
	fixtureNetwork(a)
	for b.Loop() {
		a.networkProcesses()
		a.networkConnections()
		a.mapDestinations()
	}
}
