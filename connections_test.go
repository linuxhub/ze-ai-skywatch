package main

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
	"github.com/linuxhub/ze-ai-skywatch/internal/ipnotes"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/linuxhub/ze-ai-skywatch/internal/network"
	"github.com/linuxhub/ze-ai-skywatch/internal/settings"
)

func fixtureSockets(a *app) {
	a.connections.snapshot = connections.Snapshot{At: time.Now(), Rows: []connections.Connection{
		{PID: 100, Started: 1000, Process: "Chrome", Protocol: "TCP", Family: "IPv4", State: "ESTABLISHED", Local: connections.Endpoint{IP: "192.168.1.8", Port: "53218"}, Remote: connections.Endpoint{IP: "121.43.212.142", Port: "443"}},
		{PID: 200, Started: 1000, Process: "nginx", Protocol: "TCP", Family: "IPv6", State: "LISTEN", Local: connections.Endpoint{IP: "*", Port: "8080"}},
		{PID: 300, Started: 1000, Process: "resolver", Protocol: "UDP", Family: "IPv4", Local: connections.Endpoint{IP: "192.168.1.8", Port: "53000"}, Remote: connections.Endpoint{IP: "223.5.5.5", Port: "53"}},
		{PID: 400, Started: 1000, Process: "mDNSResponder", Protocol: "UDP", Family: "IPv6", Local: connections.Endpoint{IP: "::1", Port: "5353"}},
	}}
	a.connections.snapshot.Rows[0].Traffic = network.Traffic{Download: 2048, Upload: 1024, Ready: true}
	a.connections.snapshot.Rows[0].Interface = "en0"
	a.connections.snapshot.Rows[2].Traffic = network.Traffic{Download: 512, Upload: 4096, Ready: true}
	a.connections.snapshot.Rows[2].Interface = "en1"
}
func TestSocketTrafficPublicationSearchAndSort(t *testing.T) {
	a := newApp()
	defer a.cancel()
	fixtureSockets(a)
	n := &a.connections
	n.query = "en1"
	if rows := a.filteredConnections(); len(rows) != 1 || rows[0].PID != 300 {
		t.Fatal("interface search", rows)
	}
	n.query = ""
	for _, test := range []struct {
		column     string
		descending bool
		pid        int32
	}{
		{"download", false, 300}, {"download", true, 100}, {"upload", false, 100}, {"upload", true, 300},
	} {
		n.order = ui.SortOrder{Column: test.column, Descending: test.descending}
		if rows := a.filteredConnections(); rows[0].PID != test.pid || rows[2].Ready {
			t.Fatal(test, rows)
		}
	}
	row := n.snapshot.Rows[0]
	traffic := network.Snapshot{At: time.Now(), Connections: []network.Connection{{Identity: network.Identity{PID: row.PID, Started: row.Started}, Protocol: row.Protocol, Local: row.Local.String(), Remote: row.Remote.String(), Interface: "utun3", Traffic: network.Traffic{Download: 12345, Upload: 678, Ready: true}}}}
	a.publishConnectionsTraffic(n.generation, traffic, "")
	if n.snapshot.Rows[0].Download != 12345 || n.snapshot.Rows[0].Interface != "utun3" || len(a.filteredConnections()) != 4 {
		t.Fatal("traffic update", n.snapshot)
	}
	a.publishConnectionsTraffic(n.generation+1, network.Snapshot{}, "stale")
	if n.snapshot.Rows[0].Download != 12345 || n.trafficErr != "" {
		t.Fatal("stale traffic published")
	}
	a.publishConnectionsTraffic(n.generation, network.Snapshot{}, "unavailable")
	if n.snapshot.Rows[0].Ready || n.trafficErr != "unavailable" {
		t.Fatal("failed traffic retained rates")
	}
}
func TestSocketFiltersAndNumericSort(t *testing.T) {
	a := newApp()
	defer a.cancel()
	fixtureSockets(a)
	a.config.IPNotes = []ipnotes.Note{{IP: "121.43.212.142", Name: "伊智生产环境官网", Group: "生产", Description: "业务标记"}}
	n := &a.connections
	if len(a.filteredConnections()) != 4 {
		t.Fatal("all")
	}
	n.query = "生产"
	if len(a.filteredConnections()) != 1 {
		t.Fatal("label search")
	}
	n.query = ""
	n.protocol = "UDP"
	if len(a.filteredConnections()) != 2 {
		t.Fatal("UDP")
	}
	n.mode = "监听与绑定"
	if rows := a.filteredConnections(); len(rows) != 1 || rows[0].Local.Port != "5353" {
		t.Fatal(rows)
	}
	n.protocol, n.mode, n.annotation = "全部协议", "全部连接", "已标注"
	if len(a.filteredConnections()) != 1 {
		t.Fatal("annotated")
	}
	n.annotation, n.scope = "全部标注", "本机回环"
	if len(a.filteredConnections()) != 1 {
		t.Fatal("scope")
	}
	n.scope, n.state = "全部范围", "ESTABLISHED"
	if len(a.filteredConnections()) != 1 {
		t.Fatal("state")
	}
	n.state, n.order = "全部状态", ui.SortOrder{Column: "remotePort"}
	rows := a.filteredConnections()
	if rows[2].Remote.Port != "53" || rows[3].Remote.Port != "443" {
		t.Fatal(rows)
	}
}
func TestSocketPublicationAndCancellation(t *testing.T) {
	a := newApp()
	defer a.cancel()
	fixtureSockets(a)
	n := &a.connections
	before := n.snapshot.At
	a.publishConnections(n.generation, connections.Snapshot{Err: "采集失败"})
	if len(n.snapshot.Rows) != 4 || !n.snapshot.At.Equal(before) || n.snapshot.Err == "" {
		t.Fatal("failed scan discarded last result")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	n.cancel, n.running, n.automatic = cancel, true, true
	generation := n.generation
	a.page = "connections"
	n.paused = true
	a.syncConnections()
	if ctx.Err() == nil || n.running {
		t.Fatal("pause did not cancel")
	}
	a.publishConnections(generation, connections.Snapshot{At: time.Now()})
	if !n.snapshot.At.Equal(before) || len(n.snapshot.Rows) != 4 {
		t.Fatal("stale result published")
	}
	ctx, cancel = context.WithCancel(a.ctx)
	n.cancel, n.running = cancel, true
	a.page = "ports"
	a.syncConnections()
	if ctx.Err() == nil {
		t.Fatal("leaving did not cancel")
	}
}
func TestIPNotesPersistenceValidationAndFailure(t *testing.T) {
	a := newApp()
	defer a.cancel()
	fixtureSockets(a)
	a.configPath = filepath.Join(t.TempDir(), "settings.json")
	a.editIPNote("121.43.212.142")
	a.ipNotes.draft.Name = "伊智生产环境官网"
	a.saveIPNote()
	if a.ipNotes.editing || a.ipNote("::ffff:121.43.212.142").Name != "伊智生产环境官网" {
		t.Fatal(a.ipNotes)
	}
	s, err := settings.Load(a.configPath)
	if err != nil || len(s.IPNotes) != 1 || s.IPNotes[0].Name != "伊智生产环境官网" {
		t.Fatal(s, err)
	}
	a.connections.query = "伊智"
	if len(a.filteredConnections()) != 1 {
		t.Fatal("new note did not invalidate projection")
	}
	a.editIPNote("")
	a.ipNotes.draft = ipnotes.Note{IP: "::ffff:121.43.212.142", Name: "重复"}
	a.saveIPNote()
	if a.ipNotes.err == "" || len(a.config.IPNotes) != 1 {
		t.Fatal("duplicate accepted")
	}
	a.editIPNote("121.43.212.142")
	a.ipNotes.draft.Name = "修改"
	a.configPath = t.TempDir() // rename onto directory must fail.
	a.saveIPNote()
	if !a.ipNotes.editing || a.ipNotes.err == "" || a.ipNote("121.43.212.142").Name != "伊智生产环境官网" {
		t.Fatal("failed save changed model")
	}
	a.configPath = ""
	a.deleteIPNote()
	if len(a.config.IPNotes) != 0 || a.ipNote("121.43.212.142").Name != "" || len(a.filteredConnections()) != 0 {
		t.Fatal("delete retained note/projection")
	}
}
func TestSocketUIAnnotationAndIndependentNavigation(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	fixtureSockets(a)
	tt := ui.NewTester(a.view, 1180, 850)
	tt.SetLocale("zh-Hans")
	click := func(s string) {
		t.Helper()
		if err := tt.Click(s); err != nil {
			t.Fatal(err)
		}
	}
	click("网络连接")
	if a.network.enabled || !tt.HasText("本地 IP") || !tt.HasText("远端 IP") || !tt.HasText("↓ 下载") || !tt.HasText("↑ 上传") || !tt.HasText("网卡") || !tt.HasText("2.0 KiB/s") {
		t.Fatal("page not independent")
	}
	click("Chrome")
	click("标注远端 IP")
	if a.ipNotes.draft.IP != "121.43.212.142" {
		t.Fatal(a.ipNotes.draft)
	}
	click("输入 标注名称")
	tt.Type("伊智生产环境官网")
	click("保存 IP 标注")
	if !tt.HasText("◇ 伊智生产环境官网") {
		t.Fatal("saved note not visible")
	}
	click("暂停刷新")
	if !a.connections.paused || len(a.connections.snapshot.Rows) != 4 {
		t.Fatal("pause cleared rows")
	}
	click("查看连接详情")
	if !tt.HasText("连接端点详情") || !tt.HasText("伊智生产环境官网") {
		t.Fatal("details missing")
	}
	click("关闭端点详情")
	click("管理 IP 标注")
	if !a.ipNotes.showSettings || !tt.HasText("添加 IP 标注") {
		t.Fatal("notes settings missing")
	}
	click("编辑")
	click("删除此标注")
	click("保留标注")
	click("取消标注编辑")
	click("开放端口")
	if !tt.HasText("TCP 监听端口列表") && !tt.HasText("监听地址") {
		t.Fatal("ports changed")
	}
	click("网络连接")
	click("继续刷新")
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if tt.FramePending() {
		t.Fatal("view did not settle")
	}
}
func TestSocketRenderLayouts(t *testing.T) {
	for _, width := range []int{1180, 1360} {
		for _, dark := range []bool{false, true} {
			a := fixtureApp()
			fixtureSockets(a)
			a.page = "connections"
			a.config.IPNotes = []ipnotes.Note{{IP: "121.43.212.142", Name: "伊智生产环境官网", Group: "生产", Description: "官方网站"}}
			tt := ui.NewTester(a.view, width, 850)
			tt.SetLocale("zh-Hans")
			tt.SetDark(dark)
			for _, view := range []string{"table", "selected", "detail", "stop", "process", "notes", "editor"} {
				switch view {
				case "selected":
					if err := tt.Click("Chrome"); err != nil {
						t.Fatal(err)
					}
				case "detail":
					a.connections.detail = a.connections.snapshot.Rows[0]
					a.connections.detailOpen = true
				case "stop":
					a.requestStopProcess(a.socketProcess(a.connections.detail))
				case "process":
					a.confirmStop = false
					a.openSocketProcess(a.connections.detail)
				case "notes":
					a.closeProcessDetails()
					a.connections.detailOpen = false
					a.page = "settings"
					a.ipNotes.showSettings = true
				case "editor":
					a.editIPNote("121.43.212.142")
				}
				tt.Frame()
				if tt.FramePending() {
					t.Fatalf("not settled %s", view)
				}
				if dir := os.Getenv("MAC_MONITOR_SCREENSHOTS"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("connections-%s-%d-%t.png", view, width, dark)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, tt.Image())
					_ = f.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a.cancel()
		}
	}
}

func TestSocketProcessNavigationAndStopConfirmation(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	fixtureSockets(a)
	a.page = "connections"
	tt := ui.NewTester(a.view, 1180, 700)
	tt.SetLocale("zh-Hans")
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("Chrome")
	click("查看连接详情")
	key := a.connections.detail.Key()
	click("查看进程")
	if !a.showDetail || a.connections.detailOpen || a.detail.PID != 100 || a.detail.Started != 1000 || a.detailPort != nil || a.page != "connections" {
		t.Fatal("socket did not open its process")
	}
	click("停止进程…")
	if !a.confirmStop || a.stopTarget.Identity != a.detail.Identity {
		t.Fatal("shared confirmation target")
	}
	click("取消")
	if !a.showDetail || a.connections.detailOpen {
		t.Fatal("cancel lost process details")
	}
	click("返回连接详情")
	if a.showDetail || !a.connections.detailOpen || a.connections.detail.Key() != key || a.connections.selectedKey != key {
		t.Fatal("return lost connection / selection")
	}
	a.forceStop = true
	click("结束此进程…")
	if !a.confirmStop || a.forceStop != (runtime.GOOS == "windows") || a.stopTarget.PID != 100 || !a.connections.detailOpen {
		t.Fatal("direct stop bypassed shared confirmation")
	}
	// A newly opened process pane must not retarget an existing confirmation.
	a.detail = monitor.Detail{Identity: monitor.Identity{PID: 200, Started: 999}, Name: "different process"}
	tt.Frame()
	if !tt.HasText("Chrome（PID 100）") || tt.HasText("different process（PID 200）") {
		t.Fatal("confirmation target changed")
	}
	click("取消")
	if a.confirmStop || !a.connections.detailOpen {
		t.Fatal("cancel lost socket")
	}
	click("查看进程")
	// Escape and outside dismissal also return to the originating socket.
	tt.Key(0, ui.KeyEscape)
	if !a.connections.detailOpen || a.connections.processOrigin != nil {
		t.Fatal("dismiss did not return")
	}
	click("结束此进程…")
	tt.Key(0, ui.KeyEscape)
	if a.confirmStop || !a.connections.detailOpen {
		t.Fatal("Escape did not retain socket under confirmation")
	}
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if tt.FramePending() {
		t.Fatal("navigation did not settle")
	}
}

func TestSocketProcessIdentityAndStopResults(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	fixtureSockets(a)
	row := a.connections.snapshot.Rows[0]
	// A recycled PID in the CPU sample cannot supply a different process's data.
	a.snapshot.Processes = []monitor.Process{{Identity: monitor.Identity{PID: row.PID, Started: row.Started + 1}, Name: "reused", CPU: 90, CPUReady: true}}
	if p := a.socketProcess(row); p.Name != row.Process || p.CPUReady {
		t.Fatal("reused PID matched", p)
	}
	a.snapshot.Processes[0].Started = row.Started
	if p := a.socketProcess(row); !p.CPUReady {
		t.Fatal("same identity resource sample lost")
	}
	a.page = "connections"
	a.connections.detail = row
	a.connections.detailOpen = true
	a.openSocketProcess(row)
	id := a.detail.Identity
	a.stopping = true
	a.finishProcessStop(id, errors.New("进程已退出，PID 已被其他进程使用"))
	if a.stopping || !a.showDetail || a.message == "" {
		t.Fatal("failed stop dismissed details")
	}
	a.finishProcessStop(id, nil)
	if a.showDetail || a.connections.detailOpen || a.connections.processOrigin != nil || len(a.refresh) != 1 || len(a.refreshPorts) != 1 {
		t.Fatal("stop did not close matching views and request refresh")
	}
	// A late result for the original target preserves another process's view.
	a.openProcess(monitor.Process{Identity: monitor.Identity{PID: 200, Started: 2000}, Name: "other"}, nil)
	a.finishProcessStop(id, nil)
	if !a.showDetail {
		t.Fatal("late stop result closed unrelated details")
	}
	a.confirmStop = false
	for _, p := range []monitor.Process{{Identity: monitor.Identity{PID: 1, Started: 1}}, {Identity: monitor.Identity{PID: 42}}, {Identity: monitor.Identity{PID: int32(os.Getpid()), Started: 1}}} {
		a.requestStopProcess(p)
		if a.confirmStop {
			t.Fatal("unsafe target enabled", p)
		}
	}
}
