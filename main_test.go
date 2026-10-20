package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/linuxhub/ze-ai-skywatch/internal/ports"
)

func fixtureApp() *app {
	a := newApp()
	a.snapshot = monitor.Snapshot{At: time.Now(), CPU: 38, CPUReady: true, MemoryUsed: 12 << 30, MemoryTotal: 24 << 30, MemoryReady: true, ProcessCount: 312}
	names := []string{"WindowServer", "Google Chrome", "Visual Studio Code", "kernel_task", "Spotify", "WeChat", "Docker", "Finder", "Terminal", "iTerm2"}
	for i, name := range names {
		a.snapshot.Processes = append(a.snapshot.Processes, monitor.Process{Identity: monitor.Identity{PID: int32(200 + i), Started: 1720000000000}, Name: name, CPU: float64(32 - i*3), CPUReady: true, Memory: uint64(512-i*20) << 20, MemoryReadable: true})
	}
	a.portSnapshot = ports.Snapshot{At: time.Now(), Listeners: []ports.Listener{
		{PID: 318, Process: "sshd", Protocol: "TCP", Family: "IPv4", Address: "*", Port: 22},
		{PID: 721, Process: "nginx", Protocol: "TCP", Family: "IPv4", Address: "*", Port: 80},
		{PID: 721, Process: "nginx", Protocol: "TCP", Family: "IPv6", Address: "::1", Port: 443},
		{PID: 882, Process: "mysqld", Protocol: "TCP", Family: "IPv4", Address: "127.0.0.1", Port: 3306},
		{PID: 1021, Process: "postgres", Protocol: "TCP", Family: "IPv4", Address: "127.0.0.1", Port: 5432},
		{PID: 1103, Process: "redis-server", Protocol: "TCP", Family: "IPv4", Address: "127.0.0.1", Port: 6379},
		{PID: 2231, Process: "java", Protocol: "TCP", Family: "IPv4", Address: "*", Port: 8080},
	}}
	a.history = []float64{12, 18, 15, 30, 22, 44, 26, 35, 38}
	return a
}

func TestNavigationAndSearch(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	tt := ui.NewTester(a.view, 1360, 850)
	tt.SetLocale("zh-Hans")
	if !tt.HasText("系统概览") || !tt.HasText("Google Chrome") {
		t.Fatalf("overview missing: %v", tt.Texts())
	}
	if err := tt.Click("CPU 进程"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索进程"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Chrome")
	if !tt.HasText("Google Chrome") || tt.HasText("WindowServer") {
		t.Fatal("process filtering failed")
	}
	if err := tt.Click("开放端口"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索端口"); err != nil {
		t.Fatal(err)
	}
	tt.Type("3306")
	if !tt.HasText("mysqld") || tt.HasText("nginx") {
		t.Fatal("port filtering failed")
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("刷新设置") {
		t.Fatal("settings missing")
	}
	if err := tt.Click("自动刷新监控数据"); err != nil {
		t.Fatal(err)
	}
	if a.config.AutoRefresh {
		t.Fatal("auto refresh did not pause")
	}
	if err := tt.Click("关于"); err != nil {
		t.Fatal(err)
	}
	if a.page != "about" || !tt.HasText(appAuthor) || !tt.HasText(appRepositoryURL) {
		t.Fatal("about page missing author or repository")
	}
	if len(tt.OpenedURLs()) != 0 {
		t.Fatal("about page opened a URL without a click")
	}
	if err := tt.Click(appRepositoryURL); err != nil {
		t.Fatal(err)
	}
	if urls := tt.OpenedURLs(); len(urls) != 1 || urls[0] != appRepositoryURL {
		t.Fatalf("repository link opened wrong URL: %v", urls)
	}
	if err := tt.Click("复制仓库地址"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != appRepositoryURL {
		t.Fatal("repository address was not copied")
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("刷新设置") || tt.HasText(appRepositoryURL) {
		t.Fatal("about content leaked into settings")
	}
	if err := tt.Click("关于"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(tt.OpenedURLs()) != 1 || tt.FramePending() {
		t.Fatal("returning to about did not settle or reopened the repository")
	}
}

func TestRenderLayouts(t *testing.T) {
	dir := os.Getenv("MAC_MONITOR_SCREENSHOTS")
	for _, width := range []int{1180, 1360} {
		for _, dark := range []bool{false, true} {
			a := fixtureApp()
			tt := ui.NewTester(a.view, width, 850)
			tt.SetLocale("zh-Hans")
			tt.SetDark(dark)
			for _, page := range []string{"overview", "processes", "ports", "settings", "about"} {
				a.page = page
				tt.Frame()
				if tt.FramePending() {
					t.Errorf("%s does not settle", page)
				}
				if dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					name := page + "-" + map[bool]string{true: "dark", false: "light"}[dark]
					if width == 1180 {
						name += "-compact"
					}
					f, err := os.Create(filepath.Join(dir, name+".png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, tt.Image())
					f.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a.cancel()
		}
	}
}

func TestUnavailableAndAddressFilters(t *testing.T) {
	a := newApp()
	defer a.cancel()
	if strings.Contains(cpuText(monitor.Process{}), "0%") {
		t.Fatal("unavailable CPU presented as zero")
	}
	a = fixtureApp()
	defer a.cancel()
	a.page = "ports"
	a.addressFilter = "仅本机"
	for _, p := range a.filteredPorts() {
		if !p.LocalOnly() {
			t.Fatal("wildcard leaked into loopback filter")
		}
	}
	a.page = "overview"
	if len(a.filteredPorts()) != len(a.portSnapshot.Listeners) {
		t.Fatal("overview unexpectedly uses full-page address filter")
	}
}

func TestDetailsAndStopCancellation(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	tt := ui.NewTester(a.view, 1360, 850)
	tt.SetLocale("zh-Hans")
	if err := tt.Click("WindowServer"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("进程详情"); err != nil {
		t.Fatal(err)
	}
	if !a.showDetail || !tt.HasText("可执行路径") {
		t.Fatal("process details did not open")
	}
	if err := tt.Click("停止进程…"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("确认停止此进程？") {
		t.Fatal("stop confirmation missing")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.confirmStop || !a.showDetail {
		t.Fatal("cancel must retain details and dismiss confirmation")
	}
	if dir := os.Getenv("MAC_MONITOR_SCREENSHOTS"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "process-detail-light.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	a.openPort(a.portSnapshot.Listeners[0])
	tt.Frame()
	if !a.showDetail || !tt.HasText("监听地址") {
		t.Fatal("port details did not open")
	}
	if dir := os.Getenv("MAC_MONITOR_SCREENSHOTS"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "port-detail-light.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
