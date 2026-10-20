package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/actions"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/linuxhub/ze-ai-skywatch/internal/ports"
	"github.com/linuxhub/ze-ai-skywatch/internal/settings"
)

const appName = "泽 · 观天镜"
const appEnglishName = "Ze AI SkyWatch"
const appTagline = "用 AI 看懂进程与网络"
const appAuthor = "泽泽"
const appRepositoryURL = "https://github.com/linuxhub/ze-ai-skywatch"
const appVersion = "0.4.5"

//go:embed resources/icon.png
var appIconPNG []byte

var appIcon = func() *ui.Bitmap {
	icon, err := ui.DecodeBitmap(appIconPNG)
	if err != nil {
		panic("decode application icon: " + err.Error())
	}
	return icon
}()

//go:embed resources/tray.png
var trayPNG []byte

type app struct {
	win                                                              *mygo.Window
	tray                                                             *mygo.Tray
	ctx                                                              context.Context
	cancel                                                           context.CancelFunc
	config                                                           settings.Settings
	configPath                                                       string
	workerConfig                                                     atomic.Value
	refresh, refreshPorts                                            chan struct{}
	page                                                             string
	snapshot                                                         monitor.Snapshot
	portSnapshot                                                     ports.Snapshot
	history                                                          []float64
	sampling, scanning                                               bool
	query, portQuery, addressFilter, interval                        string
	processList, topList, portList, overviewPortList                 ui.ListState
	processSelected, topSelected, portSelected, overviewPortSelected int
	processSort, portSort                                            ui.SortOrder
	showDetail, detailLoading                                        bool
	detail                                                           monitor.Detail
	detailProcess                                                    monitor.Process
	detailPort                                                       *ports.Listener
	detailGeneration                                                 uint64
	confirmStop, forceStop, stopping                                 bool
	stopTarget                                                       monitor.Process
	message                                                          string
	messageUntil                                                     time.Time
	loginEnabled                                                     bool
	quitting                                                         bool
	providerUI                                                       providerState
	analysis                                                         analysisState
	detailAt                                                         time.Time
	detailInspectError                                               string
	network                                                          networkState
	connections                                                      connectionsState
	ipNotes                                                          ipNotesState
}

func newApp() *app {
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{ctx: ctx, cancel: cancel, config: settings.Default(), page: "overview", interval: "2 秒", addressFilter: "全部地址",
		refresh: make(chan struct{}, 1), refreshPorts: make(chan struct{}, 1),
		processSelected: -1, topSelected: -1, portSelected: -1, overviewPortSelected: -1,
		processSort: ui.SortOrder{Column: "cpu", Descending: true}, portSort: ui.SortOrder{Column: "port"}}
	a.workerConfig.Store(a.config)
	a.initNetwork()
	a.initConnections()
	return a
}

func main() {
	// Keep existing settings while the display and executable names change.
	dataName := "观天镜"
	if executable, err := os.Executable(); err == nil && strings.HasSuffix(strings.TrimSuffix(filepath.Base(executable), ".exe"), " Dev") {
		dataName += " Dev"
	}
	if dir, err := os.UserConfigDir(); err == nil {
		mygo.App.SetPath(mygo.PathUserData, filepath.Join(dir, dataName))
	}
	mygo.App.SetName(appName)
	mygo.App.SetLanguages("zh-Hans")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := newApp()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true; a.cancel() })
	mygo.App.OnQuit(a.cancel)
	mygo.App.OnSecondInstance(func([]string, string) { a.showWindow() })
	mygo.App.OnActivate(func(bool) { a.showWindow() })
	mygo.App.WhenReady(func() {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err == nil {
			a.configPath = filepath.Join(dir, "settings.json")
			if a.config, err = settings.Load(a.configPath); err != nil {
				a.notify("读取设置失败，已使用默认设置: " + err.Error())
			}
		}
		a.workerConfig.Store(a.config)
		a.interval = fmt.Sprintf("%d 秒", a.config.Interval)
		a.loginEnabled = mygo.App.OpenAtLogin()
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title: appName, Width: 1360, Height: 850, MinWidth: 1180, MinHeight: 700,
			TitleBarStyle: mygo.TitleBarHiddenInset, TitleBarHeight: 56,
			BackgroundColor: "light-dark(#f6f8fb, #171b24)", StateKey: "main",
			Hidden:  mygo.App.WasOpenedAtLogin() && a.config.StartHidden && a.config.Tray,
			Content: ui.View(a.view),
		})
		a.configureTray()
		if a.tray == nil {
			a.win.Show()
		}
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting && a.tray != nil {
				e.PreventDefault()
				a.win.Hide()
			}
		})
		a.win.OnClosed(a.cancel)
		a.win.OnHide(func() { a.network.visible = false; a.syncConnections() })
		a.win.OnMinimize(func() { a.network.visible = false; a.syncConnections() })
		a.win.OnShow(func() { a.network.visible = true; a.syncConnections() })
		a.win.OnRestore(func() { a.network.visible = true; a.syncConnections() })
		a.network.visible = a.win.IsVisible() && !a.win.IsMinimized()
		a.startWorkers()
	})
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func (a *app) showWindow() {
	if a.win != nil && !a.win.IsDestroyed() {
		a.win.Show()
		a.win.Restore()
		a.win.Focus()
	}
}
func (a *app) notify(text string) { a.message = text; a.messageUntil = time.Now().Add(7 * time.Second) }

func (a *app) configureTray() {
	if !a.config.Tray {
		if a.tray != nil {
			a.tray.Destroy()
			a.tray = nil
		}
		return
	}
	if a.tray != nil {
		return
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: trayPNG, IconIsTemplate: false, ToolTip: appName,
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开" + appName, Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			{Label: "立即刷新", Click: func(*mygo.MenuItem, *mygo.Window) { a.requestRefresh() }},
			mygo.Separator(), {Role: mygo.RoleQuit},
		}),
	})
	if err != nil {
		a.notify("菜单栏图标不可用: " + err.Error())
		return
	}
	a.tray = tray
}

func (a *app) persist() {
	a.workerConfig.Store(a.config)
	if a.configPath != "" {
		if err := settings.Save(a.configPath, a.config); err != nil {
			a.notify("保存设置失败: " + err.Error())
		}
	}
}

func send(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
func (a *app) requestRefresh() {
	a.refreshConnections()
	if a.refresh != nil {
		send(a.refresh)
		send(a.refreshPorts)
	}
}

// Only these workers own the collectors. Window.Update publishes snapshots on
// the UI thread, so rendering never waits for lsof or reads worker-owned state.
func (a *app) startWorkers() {
	a.startNetworkLight()
	go func() {
		sampler := monitor.NewSampler()
		timer := time.NewTicker(250 * time.Millisecond)
		defer timer.Stop()
		var last time.Time
		samples := 0
		collect := func() {
			a.win.Update(func() { a.sampling = true })
			s := sampler.Sample(a.ctx)
			if a.ctx.Err() != nil {
				return
			}
			a.win.Update(func() {
				a.snapshot = s
				a.sampling = false
				if s.CPUReady {
					a.history = append(a.history, s.CPU)
					if len(a.history) > 48 {
						a.history = a.history[len(a.history)-48:]
					}
				}
				if a.tray != nil && s.CPUReady {
					a.tray.SetTitle(fmt.Sprintf("%.0f%%", s.CPU))
				}
			})
			samples++
			last = time.Now()
		}
		collect()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-a.refresh:
				collect()
			case <-timer.C:
				cfg := a.workerConfig.Load().(settings.Settings)
				if (cfg.AutoRefresh || samples < 2) && time.Since(last) >= time.Duration(cfg.Interval)*time.Second {
					collect()
				}
			}
		}
	}()
	go func() {
		timer := time.NewTicker(time.Second)
		defer timer.Stop()
		last := time.Time{}
		collect := func() {
			a.win.Update(func() { a.scanning = true })
			s := ports.Scan(a.ctx)
			if a.ctx.Err() != nil {
				return
			}
			a.win.Update(func() {
				// Retain successful rows and their timestamp when a later scan fails.
				if s.Err == "" {
					a.portSnapshot = s
				} else {
					a.portSnapshot.Err = s.Err
				}
				a.scanning = false
			})
			last = time.Now()
		}
		collect()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-a.refreshPorts:
				collect()
			case <-timer.C:
				cfg := a.workerConfig.Load().(settings.Settings)
				if cfg.AutoRefresh && time.Since(last) >= time.Duration(max(5, cfg.Interval))*time.Second {
					collect()
				}
			}
		}
	}()
}

func (a *app) openProcess(p monitor.Process, port *ports.Listener) {
	a.connections.processOrigin = nil
	a.cancelAnalysis()
	a.analysis = analysisState{tab: "进程信息"}
	a.detailGeneration++
	generation := a.detailGeneration
	a.detailProcess = p
	a.detailPort = port
	a.detailAt = a.snapshot.At
	a.detailInspectError = ""
	a.showDetail = true
	a.detailLoading = true
	a.detail = monitor.Detail{Identity: p.Identity, Name: p.Name}
	if a.win == nil {
		a.detailLoading = false
		return
	}
	go func() {
		d, err := monitor.Inspect(a.ctx, p.Identity)
		a.win.Update(func() {
			if generation != a.detailGeneration {
				return
			}
			a.detailLoading = false
			if err != nil {
				a.detailInspectError = err.Error()
				a.detail.Errors = []string{err.Error()}
				a.analysis.err = err.Error()
				a.analysis.pending = false
			} else {
				a.detail = d
				if a.analysis.pending && a.showDetail {
					a.analysis.pending = false
					a.startAnalysis()
				}
			}
		})
	}()
}

func canStopProcess(p monitor.Process) bool {
	minimumPID := int32(1)
	if runtime.GOOS == "windows" {
		minimumPID = 4
	}
	return p.PID > minimumPID && p.PID != int32(os.Getpid()) && p.Started > 0
}

// Capture the target before confirmation. Other detail panes and subsequent
// samples must never change the identity that the user is confirming.
func (a *app) requestStopProcess(p monitor.Process) {
	if a.stopping || !canStopProcess(p) {
		return
	}
	a.stopTarget = p
	a.forceStop = runtime.GOOS == "windows"
	a.confirmStop = true
}

func (a *app) stopProcess() {
	id := a.stopTarget.Identity
	force := a.forceStop
	a.confirmStop = false
	a.stopping = true
	go func() {
		err := actions.Terminate(a.ctx, id, force)
		a.win.Update(func() {
			a.finishProcessStop(id, err)
		})
	}()
}

func (a *app) finishProcessStop(id monitor.Identity, err error) {
	a.stopping = false
	if err != nil {
		a.notify(err.Error())
		return
	}
	if a.detail.Identity == id {
		a.cancelAnalysis()
		a.showDetail = false
	}
	n := &a.connections
	if origin := n.processOrigin; origin != nil && origin.PID == id.PID && origin.Started == id.Started {
		n.processOrigin = nil
	}
	if n.detail.PID == id.PID && n.detail.Started == id.Started {
		n.detailOpen = false
	}
	a.notify(platformText("已发送停止信号，正在刷新连接、进程和端口", "进程已结束，正在刷新连接、进程和端口"))
	a.requestRefresh()
}

func (a *app) closeProcessDetails() {
	a.cancelAnalysis()
	a.showDetail = false
	if origin := a.connections.processOrigin; origin != nil {
		if a.page == "connections" {
			a.connections.detail = *origin
			a.connections.detailOpen = true
		}
		a.connections.processOrigin = nil
	}
}
