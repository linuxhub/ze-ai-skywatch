package main

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/linuxhub/ze-ai-skywatch/internal/ports"
)

var (
	blue         = ui.Hex("#1677ff")
	purple       = ui.Hex("#8254f8")
	green        = ui.Hex("#16a878")
	red          = ui.Hex("#f26673")
	pulseIcon    = icon(`<path d="M2 12h5l3-8 4 16 3-8h5"/>`)
	cpuIcon      = icon(`<rect x="6" y="6" width="12" height="12" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v4m6-4v4M9 18v4m6-4v4M2 9h4m-4 6h4m12-6h4m-4 6h4"/>`)
	portsIcon    = icon(`<circle cx="6" cy="6" r="3"/><circle cx="18" cy="18" r="3"/><path d="m8 8 8 8M6 9v5a4 4 0 0 0 4 4h5M18 15v-5a4 4 0 0 0-4-4H9"/>`)
	networkIcon  = icon(`<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a16 16 0 0 1 0 18 16 16 0 0 1 0-18"/>`)
	settingsIcon = icon(`<path d="m9 3 1-1h4l1 3 3 1 3 2-1 3v2l1 3-3 2-3 1-1 3h-4l-1-3-3-1-3-2 1-3v-2L3 8l3-2 3-1z"/><circle cx="12" cy="12" r="3"/>`)
	infoIcon     = icon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10h.01"/>`)
	refreshIcon  = icon(`<path d="M20 8a8 8 0 1 0 0 8M20 3v5h-5"/>`)
	arrowIcon    = icon(`<path d="m9 5 7 7-7 7"/>`)
	memoryIcon   = icon(`<rect x="3" y="6" width="18" height="12" rx="2"/><path d="M7 10v4m5-4v4m5-4v4M7 18v3m5-3v3m5-3v3"/>`)
	copyIcon     = icon(`<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>`)
	aiIcon       = icon(`<path d="m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5zM20 2v4m-2-2h4"/>`)
)

func icon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}
func bytesText(n uint64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	}
	if n >= 1<<20 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.0f KB", float64(n)/1024)
}
func cpuText(p monitor.Process) string {
	if !p.CPUReady {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", p.CPU)
}
func memoryText(p monitor.Process) string {
	if !p.MemoryReadable {
		return "—"
	}
	return bytesText(p.Memory)
}
func placeholder(s string) string {
	if s == "" {
		return "不可用"
	}
	return s
}
func (a *app) palette(c *ui.Context) (surface, background ui.Color) {
	if c.Theme().Dark {
		return ui.Hex("#202632"), ui.Hex("#171b24")
	}
	return ui.Hex("#ffffff"), ui.Hex("#f6f8fb")
}

func (a *app) view(c *ui.Context) {
	a.syncConnections()
	t := *c.Theme()
	t.Accent = blue
	t.Radius = 7
	t.Spacing = 4
	t.FontSize = 13
	c.SetTheme(&t)
	surface, bg := a.palette(c)
	c.Root().Background(bg)
	ui.Column(c).Fill().Children(func() {
		a.titleBar(c, surface)
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			ui.Column(c).Grow(1).MinWidth(0).Padding(24).Gap(20).Children(func() {
				switch a.page {
				case "settings":
					a.settingsView(c)
				case "about":
					a.aboutPage(c)
				default:
					a.pageHeader(c)
					switch a.page {
					case "processes":
						a.processPage(c)
					case "ports":
						a.portsPage(c)
					case "network":
						a.networkPage(c)
					case "connections":
						a.connectionsPage(c)
					default:
						a.overview(c)
					}
				}
			})
		})
		a.statusBar(c)
	})
	a.detailsView(c)
	if !a.showDetail && a.connections.processOrigin != nil {
		a.closeProcessDetails()
	}
	if !a.showDetail && (a.analysis.loading || a.analysis.pending) {
		a.cancelAnalysis()
	}
	a.providerEditor(c)
	a.networkDetailView(c)
	a.socketDetailView(c)
	a.ipNoteEditor(c)
	a.confirmView(c)
	if a.message != "" && time.Now().Before(a.messageUntil) {
		c.After(time.Until(a.messageUntil))
		ui.Row(c).Absolute().Left(230).Bottom(46).Right(24).Padding(13, 18).Radius(10).
			Background(t.Text).TextColor(t.Background).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.12)).Children(func() {
			ui.Text(c, a.message).Grow(1)
			if ui.Button(c, "关闭提示").Clicked() {
				a.message = ""
			}
		})
	}
}

func (a *app) titleBar(c *ui.Context, surface ui.Color) {
	t := c.Theme()
	left := float32(96)
	if runtime.GOOS == "windows" {
		left = 20
	}
	if c.TitleBar().Left > left {
		left = c.TitleBar().Left + 16
	}
	right := float32(20)
	if c.TitleBar().Right > 0 {
		right = c.TitleBar().Right + 16
	}
	ui.Row(c).Height(56).Shrink(0).Padding(0, right, 0, left).Gap(10).Background(surface).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).DragWindow().Children(func() {
		ui.Image(c, appIcon).Size(28, 28).Fit(ui.Contain)
		ui.Text(c, appName).Bold().FontSize(14)
		ui.Text(c, appTagline).TextColor(t.TextMuted).Margin(0, 0, 0, 6)
		ui.Spacer(c)
		label := "尚未采样"
		if !a.snapshot.At.IsZero() {
			label = "最后刷新  " + a.snapshot.At.Format("15:04:05")
		}
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		a.iconButton(c, "refresh", "立即刷新", refreshIcon, a.requestRefresh)
		a.iconButton(c, "settings", "打开设置", settingsIcon, func() { a.page = "settings" })
	})
}

func (a *app) iconButton(c *ui.Context, key, label string, i *ui.SVG, fn func()) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Size(32, 30).Radius(6).Center().Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Icon(c, i).Size(17, 17).TextColor(t.TextMuted) })
	if b.Clicked() {
		fn()
	}
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	_, bg := a.palette(c)
	ui.Column(c).Width(194).Shrink(0).Background(bg).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Padding(22, 14).Gap(6).Children(func() {
		ui.Text(c, "工作空间").FontSize(10).Bold().TextColor(t.TextMuted).Padding(0, 10, 12)
		for _, nav := range []struct {
			id, label string
			svg       *ui.SVG
		}{{"overview", "系统概览", pulseIcon}, {"processes", "CPU 进程", cpuIcon}, {"network", "网络流量", networkIcon}, {"connections", "网络连接", portsIcon}, {"ports", "开放端口", portsIcon}, {"settings", "设置", settingsIcon}, {"about", "关于", infoIcon}} {
			active := a.page == nav.id
			b := ui.ButtonBase(c.Key(nav.id)).FillWidth().Height(42).Padding(0, 12).Radius(8).Label(nav.label).Gap(11)
			color := t.Text
			if active {
				b.Background(blue).Shadow(0, 3, 8, 0, blue.Alpha(0.15))
				color = ui.Hex("#ffffff")
			} else if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() { ui.Icon(c, nav.svg).Size(18, 18).TextColor(color); ui.Text(c, nav.label).TextColor(color) })
			if b.Clicked() {
				a.page = nav.id
			}
		}
		ui.Spacer(c)
		ui.Column(c).Padding(12).Gap(8).Radius(9).Background(t.Surface).Children(func() {
			ui.Row(c).Gap(6).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(green)
				ui.Text(c, "本机监控").Bold().FontSize(11)
			})
			architecture := platformText("Apple Silicon", "ARM64")
			if runtime.GOARCH == "amd64" {
				architecture = platformText("Intel Mac", "x64")
			}
			ui.Text(c, platformName()+" · "+architecture).FontSize(10).TextColor(t.TextMuted)
			mode := "已暂停自动刷新"
			if a.config.AutoRefresh {
				mode = fmt.Sprintf("每 %d 秒采样", a.config.Interval)
			}
			ui.Text(c, mode).FontSize(10).TextColor(t.TextMuted)
		})
		ui.Text(c, appName+"  /  "+appVersion).FontSize(9).TextColor(t.TextMuted).Padding(12, 8, 0)
	})
}

func (a *app) pageHeader(c *ui.Context) {
	title, subtitle := "系统概览", platformText("了解此刻的 Mac，找到繁忙的进程和正在监听的服务。", "了解此刻的电脑，找到繁忙的进程和正在监听的服务。")
	if a.page == "processes" {
		title = "CPU 进程"
		subtitle = "按 CPU 占用查看进程，双击行或选择后查看详情。"
	}
	if a.page == "ports" {
		title = "开放端口"
		subtitle = "查看本机 TCP 监听地址，以及占用它们的进程。"
	}
	if a.page == "network" {
		title = "网络流量"
		subtitle = "日常轻量看网速，需要时再追踪进程与连接去向。"
	}
	if a.page == "connections" {
		title = "网络连接"
		subtitle = "查看 TCP / UDP 两端地址、连接状态和你的 IP 业务标注。"
	}
	ui.Row(c).Shrink(0).Gap(12).Children(func() {
		ui.Column(c).Grow(1).Gap(6).Children(func() {
			ui.Text(c, title).FontSize(25).Bold()
			ui.Text(c, subtitle).FontSize(12).TextColor(c.Theme().TextMuted)
		})
		label := "自动刷新中"
		color := green
		if !a.config.AutoRefresh {
			label = "自动刷新已暂停"
			color = c.Theme().TextMuted
		}
		if a.page == "network" {
			label = "轻量模式 · 每 2 秒更新"
			if a.network.enabled {
				label = "详细分析中 · 每 2 秒采样"
			}
			color = green
		}
		if a.page == "connections" {
			label, color = "连接 5 秒 · 速率 2 秒", green
			if a.connections.paused {
				label, color = "连接刷新已暂停", c.Theme().TextMuted
			}
		}
		ui.Row(c).Gap(6).Padding(6, 10).Radius(999).Background(color.Alpha(0.09)).Children(func() {
			ui.Box(c).Size(5, 5).Radius(3).Background(color)
			ui.Text(c, label).FontSize(11).TextColor(color)
		})
	})
}

func (a *app) overview(c *ui.Context) {
	a.statCards(c)
	a.networkOverview(c)
	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Gap(16).Children(func() {
		a.panel(c, func() {
			ui.Row(c).Padding(16, 16, 12).Gap(9).Children(func() {
				ui.Icon(c, cpuIcon).Size(18, 18).TextColor(blue)
				ui.Text(c, "CPU 进程使用率").Bold().FontSize(14)
				ui.Text(c, "TOP 10").FontSize(9).Bold().TextColor(blue).Padding(3, 6).Radius(4).Background(blue.Alpha(0.08))
				ui.Spacer(c)
				if ui.Button(c, "查看全部").FontSize(11).Clicked() {
					a.page = "processes"
				}
			})
			rows := a.snapshot.Processes
			if len(rows) > 10 {
				rows = rows[:10]
			}
			a.processTable(c, rows, true)
			ui.Row(c).Padding(10, 16).Gap(8).Children(func() {
				ui.Text(c, "100% = 占满一个逻辑核心").FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
				if ui.Button(c, "进程详情").FontSize(11).Disabled(a.topSelected < 0 || a.topSelected >= len(rows)).Clicked() {
					a.openProcess(rows[a.topSelected], nil)
				}
			})
		})
		a.panel(c, func() {
			ui.Row(c).Padding(16, 16, 12).Gap(9).Children(func() {
				ui.Icon(c, portsIcon).Size(18, 18).TextColor(blue)
				ui.Text(c, "本机开放端口").Bold().FontSize(14)
				ui.Spacer(c)
				if ui.Button(c, "查看全部端口").FontSize(11).Clicked() {
					a.page = "ports"
				}
			})
			ui.SearchField(c.Key("overview-port-search"), &a.portQuery).Placeholder("搜索端口、进程名称或 PID…").Label("搜索开放端口").Margin(0, 14, 12)
			rows := a.filteredPorts()
			a.portTable(c, rows, true)
			ui.Row(c).Padding(10, 16).Gap(6).Children(func() {
				ui.Textf(c, "%d 个监听端点", len(rows)).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
				if ui.Button(c, "端口详情").FontSize(11).Disabled(a.overviewPortSelected < 0 || a.overviewPortSelected >= len(rows)).Clicked() {
					a.openPort(rows[a.overviewPortSelected])
				}
			})
		})
	})
}

func (a *app) panel(c *ui.Context, children func()) {
	surface, _ := a.palette(c)
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Radius(11).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).
		Shadow(0, 2, 7, 0, ui.RGBA(0, 0, 0, 0.025)).Clip().Children(children)
}

func (a *app) statCards(c *ui.Context) {
	t := c.Theme()
	surface, _ := a.palette(c)
	cpuValue := "—"
	if a.snapshot.CPUReady {
		cpuValue = fmt.Sprintf("%.0f%%", a.snapshot.CPU)
	}
	memoryValue := "—"
	ratio := float64(0)
	if a.snapshot.MemoryReady && a.snapshot.MemoryTotal > 0 {
		ratio = float64(a.snapshot.MemoryUsed) / float64(a.snapshot.MemoryTotal)
		memoryValue = fmt.Sprintf("%.0f%%", ratio*100)
	}
	processesValue := "—"
	if !a.snapshot.At.IsZero() {
		processesValue = strconv.Itoa(a.snapshot.ProcessCount)
	}
	portValue := "—"
	if !a.portSnapshot.At.IsZero() {
		portValue = strconv.Itoa(len(a.portSnapshot.Listeners))
	}
	ui.Row(c).Height(132).Shrink(0).Gap(14).AlignItems(ui.Stretch).Children(func() {
		for index, card := range []struct {
			title, value, caption string
			color                 ui.Color
			svg                   *ui.SVG
		}{
			{"CPU 使用率", cpuValue, "整机使用率", blue, cpuIcon},
			{"运行进程", processesValue, "当前可枚举的进程", green, pulseIcon},
			{"内存使用率", memoryValue, bytesText(a.snapshot.MemoryUsed) + " / " + bytesText(a.snapshot.MemoryTotal), purple, memoryIcon},
			{"TCP 监听端点", portValue, "按进程、地址与端口计数", blue, portsIcon},
		} {
			ui.Column(c.Key(index)).Grow(1).Basis(0).MinWidth(0).Padding(17, 18).Gap(8).Radius(10).Background(surface).Border(1, t.Border.Alpha(0.7)).Children(func() {
				ui.Row(c).Children(func() {
					ui.Text(c, card.title).FontSize(12).TextColor(t.TextMuted).Grow(1)
					ui.Icon(c, card.svg).Size(15, 15).TextColor(card.color)
				})
				ui.Text(c, card.value).FontSize(29).Bold()
				switch index {
				case 0:
					a.usageBar(c, a.snapshot.CPU/100, blue, 5)
				case 1:
					a.sparkline(c, a.history, green)
				case 2:
					a.usageBar(c, ratio, purple, 5)
				case 3:
					ui.Row(c).Height(5).Gap(3).Children(func() {
						for i := 0; i < 14; i++ {
							alpha := float32(0.15)
							if i < min(len(a.portSnapshot.Listeners), 14) {
								alpha = 0.75
							}
							ui.Box(c).Grow(1).Height(5).Radius(2).Background(blue.Alpha(alpha))
						}
					})
				}
				ui.Text(c, card.caption).FontSize(9).TextColor(t.TextMuted).SingleLine()
			})
		}
	})
}

func (a *app) usageBar(c *ui.Context, value float64, color ui.Color, height float32) {
	ui.Box(c).Height(height).Radius(height / 2).Background(color.Alpha(0.11)).Clip().Children(func() {
		ui.Box(c).Height(height).WidthPercent(float32(math.Max(0, math.Min(1, value))) * 100).Radius(height / 2).Background(color)
	})
}

func (a *app) sparkline(c *ui.Context, values []float64, color ui.Color) {
	ui.Box(c).Height(5).Draw(func(p *ui.Painter, r ui.Rect) {
		if len(values) < 2 {
			p.Line(r.X, r.Y+2, r.X+r.W, r.Y+2, 1, color.Alpha(0.25))
			return
		}
		var path ui.Path
		for i, v := range values {
			x := r.X + float32(i)*r.W/float32(len(values)-1)
			y := r.Y + r.H - float32(v/100)*r.H
			if i == 0 {
				path.MoveTo(x, y)
			} else {
				path.LineTo(x, y)
			}
		}
		p.StrokePath(&path, 1.5, color)
	})
}

func (a *app) processTable(c *ui.Context, rows []monitor.Process, compact bool) {
	state, selected := &a.processList, &a.processSelected
	if compact {
		state, selected = &a.topList, &a.topSelected
	}
	state.Selected = selected
	state.KeepOffset = true
	state.Key = func(i int) any { return rows[i].Identity }
	state.Label = func(i int) string { return rows[i].Name }
	columns := []ui.TableColumn{
		{Title: "#", Width: 30, Fixed: true}, {Title: "进程名称", ID: "name", MinWidth: 110, Sortable: !compact},
		{Title: "PID", ID: "pid", Width: 60, Sortable: !compact}, {Title: "CPU 使用率", ID: "cpu", Width: 116, Sortable: !compact},
		{Title: "内存 RSS", ID: "memory", Width: 92, Align: ui.End, Sortable: !compact},
		{Title: "AI", Width: 40, Fixed: true},
	}
	if compact {
		columns[3].Title = "CPU"
		columns[0].Width = 24
		columns[2].Width = 50
		columns[3].Width = 86
		columns[4].Width = 78
	}
	if !compact {
		state.Sort = &a.processSort
	}
	table := ui.Table(c, state, columns, len(rows), func(row, col int) {
		p := rows[row]
		switch col {
		case 0:
			ui.Textf(c, "%d", row+1).FontSize(11).TextColor(c.Theme().TextMuted)
		case 1:
			ui.Row(c).Gap(8).Children(func() {
				name := []rune(p.Name)
				letter := "?"
				if len(name) > 0 {
					letter = strings.ToUpper(string(name[:1]))
				}
				ui.Box(c).Size(22, 22).Radius(5).Background(blue.Alpha(0.08)).Center().Children(func() { ui.Text(c, letter).FontSize(10).Bold().TextColor(blue) })
				ui.Text(c, p.Name).SingleLine().Tooltip(p.Name).FontSize(12)
			})
		case 2:
			ui.Textf(c, "%d", p.PID).FontSize(11)
		case 3:
			ui.Box(c).FillWidth().Height(21).Radius(4).Background(red.Alpha(0.08)).Clip().Children(func() {
				if p.CPUReady {
					ui.Box(c).Absolute().Top(0).Left(0).Bottom(0).WidthPercent(float32(math.Min(p.CPU, 100))).Background(red.Alpha(0.28)).Radius(4)
				}
				ui.Text(c, cpuText(p)).Absolute().Left(6).Top(3).FontSize(11)
			}).Tooltip("100% 表示占满一个逻辑核心；多核进程可以超过 100%")
		case 4:
			ui.Text(c, memoryText(p)).FontSize(11)
		case 5:
			a.aiButton(c, fmt.Sprintf("分析进程 %s PID %d", p.Name, p.PID), func() { a.openProcessAI(p, nil) })
		}
	}).Grow(1).MinHeight(0).Label("CPU 进程列表")
	if table.Submitted() && *selected >= 0 && *selected < len(rows) {
		a.openProcess(rows[*selected], nil)
	}
	if len(rows) == 0 {
		table.Children(func() {
			ui.Text(c, "暂无进程数据，或没有匹配的进程").Padding(20).TextColor(c.Theme().TextMuted)
		})
	}
	if !compact && table.Changed() {
		c.Invalidate()
	}
}

func (a *app) processPage(c *ui.Context) {
	a.panel(c, func() {
		ui.Row(c).Padding(16).Gap(12).Children(func() {
			ui.SearchField(c.Key("process-search"), &a.query).Placeholder("搜索进程名称或 PID…").Label("搜索进程").Grow(1)
			ui.Textf(c, "%d 个进程", len(a.snapshot.Processes)).FontSize(11).TextColor(c.Theme().TextMuted)
		})
		rows := a.filteredProcesses()
		a.processTable(c, rows, false)
		ui.Row(c).Padding(12, 16).Children(func() {
			ui.Text(c, "CPU：100% 为一个核心  ·  内存：驻留集 RSS").FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
			if ui.Button(c, "查看进程详情").Disabled(a.processSelected < 0 || a.processSelected >= len(rows)).Clicked() {
				a.openProcess(rows[a.processSelected], nil)
			}
		})
	})
}

func (a *app) filteredProcesses() []monitor.Process {
	rows := make([]monitor.Process, 0, len(a.snapshot.Processes))
	q := strings.ToLower(strings.TrimSpace(a.query))
	for _, p := range a.snapshot.Processes {
		if q == "" || strings.Contains(strings.ToLower(p.Name), q) || strings.Contains(strconv.Itoa(int(p.PID)), q) {
			rows = append(rows, p)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		x, y := rows[i], rows[j]
		cmp := 0
		switch a.processSort.Column {
		case "name":
			cmp = strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		case "pid":
			cmp = compare(float64(x.PID), float64(y.PID))
		case "memory":
			if x.MemoryReadable != y.MemoryReadable {
				return x.MemoryReadable
			}
			cmp = compare(float64(x.Memory), float64(y.Memory))
		default:
			if x.CPUReady != y.CPUReady {
				return x.CPUReady
			}
			cmp = compare(x.CPU, y.CPU)
		}
		if cmp == 0 {
			return x.PID < y.PID
		}
		if a.processSort.Descending {
			return cmp > 0
		}
		return cmp < 0
	})
	return rows
}
func compare(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func (a *app) filteredPorts() []ports.Listener {
	rows := []ports.Listener{}
	q := strings.ToLower(strings.TrimSpace(a.portQuery))
	for _, p := range a.portSnapshot.Listeners {
		if a.page == "ports" {
			if a.addressFilter == "仅本机" && !p.LocalOnly() {
				continue
			}
			if a.addressFilter == "非回环地址" && p.LocalOnly() {
				continue
			}
		}
		haystack := fmt.Sprintf("%d %d %s %s %s %s", p.Port, p.PID, p.Process, p.Protocol, p.Address, p.Family)
		if q == "" || strings.Contains(strings.ToLower(haystack), q) {
			rows = append(rows, p)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		x, y := rows[i], rows[j]
		cmp := 0
		switch a.portSort.Column {
		case "process":
			cmp = strings.Compare(x.Process, y.Process)
		case "pid":
			cmp = compare(float64(x.PID), float64(y.PID))
		case "address":
			cmp = strings.Compare(x.Address, y.Address)
		default:
			cmp = compare(float64(x.Port), float64(y.Port))
		}
		if cmp == 0 {
			return x.Key() < y.Key()
		}
		if a.portSort.Descending {
			return cmp > 0
		}
		return cmp < 0
	})
	return rows
}

func (a *app) portTable(c *ui.Context, rows []ports.Listener, compact bool) {
	state, selected := &a.portList, &a.portSelected
	if compact {
		state, selected = &a.overviewPortList, &a.overviewPortSelected
	}
	state.Selected = selected
	state.KeepOffset = true
	state.Key = func(i int) any { return rows[i].Key() }
	state.Label = func(i int) string { return rows[i].Endpoint() + " " + rows[i].Process }
	columns := []ui.TableColumn{
		{Title: "端口", ID: "port", Width: 60, Sortable: !compact}, {Title: "协议", Width: 48},
		{Title: "监听地址", ID: "address", Width: 128, Sortable: !compact},
		{Title: "进程名称", ID: "process", MinWidth: 96, Sortable: !compact},
		{Title: "PID", ID: "pid", Width: 58, Sortable: !compact},
	}
	if !compact {
		state.Sort = &a.portSort
		columns = append(columns, ui.TableColumn{Title: "状态", Width: 80})
	}
	columns = append(columns, ui.TableColumn{Title: "AI", Width: 40, Fixed: true})
	if compact {
		columns[0].Width = 48
		columns[1].Width = 48
		columns[2].Width = 100
		columns[3].MinWidth = 72
		columns[4].Width = 48
	}
	table := ui.Table(c, state, columns, len(rows), func(row, col int) {
		p := rows[row]
		if col == len(columns)-1 {
			a.aiButton(c, fmt.Sprintf("分析端口 %d PID %d", p.Port, p.PID), func() { a.openPortAI(p) })
			return
		}
		switch col {
		case 0:
			ui.Textf(c, "%d", p.Port).FontSize(12).Bold()
		case 1:
			ui.Text(c, p.Protocol).FontSize(10).TextColor(c.Theme().TextMuted)
		case 2:
			ui.Row(c).Gap(5).Children(func() {
				color := blue
				if p.LocalOnly() {
					color = green
				}
				ui.Box(c).Size(5, 5).Radius(3).Background(color)
				ui.Text(c, p.Address).FontSize(11).SingleLine().Tooltip(p.Family + " · " + p.Endpoint())
			})
		case 3:
			ui.Text(c, p.Process).FontSize(12).SingleLine().Tooltip(p.Process)
		case 4:
			ui.Textf(c, "%d", p.PID).FontSize(11)
		case 5:
			ui.Text(c, "● 监听中").FontSize(11).TextColor(green)
		}
	}).Grow(1).MinHeight(0).Label("TCP 监听端口列表")
	if table.Submitted() && *selected >= 0 && *selected < len(rows) {
		a.openPort(rows[*selected])
	}
	if len(rows) == 0 {
		table.Children(func() {
			ui.Text(c, "暂无可见的 TCP 监听端口，或没有匹配项").Padding(20).TextColor(c.Theme().TextMuted)
		})
	}
	if !compact && table.Changed() {
		c.Invalidate()
	}
}

func (a *app) portsPage(c *ui.Context) {
	a.panel(c, func() {
		ui.Row(c).Padding(16).Gap(12).Children(func() {
			ui.SearchField(c.Key("port-search"), &a.portQuery).Placeholder("搜索端口、进程、PID 或地址…").Label("搜索端口").Grow(1)
			ui.Select(c, &a.addressFilter, []string{"全部地址", "仅本机", "非回环地址"}).Width(140).Label("监听地址筛选")
		})
		rows := a.filteredPorts()
		a.portTable(c, rows, false)
		ui.Row(c).Padding(12, 16).Children(func() {
			ui.Textf(c, "%d 个监听端点 · IPv4 / IPv6 分别计数", len(rows)).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
			if ui.Button(c, "查看端口详情").Disabled(a.portSelected < 0 || a.portSelected >= len(rows)).Clicked() {
				a.openPort(rows[a.portSelected])
			}
		})
	})
	ui.Text(c, "回环地址仅供本机连接；通配或网卡地址的监听是否可远程访问，还取决于防火墙与网络设置。").FontSize(11).TextColor(c.Theme().TextMuted)
}

func (a *app) openPort(p ports.Listener) {
	for _, proc := range a.snapshot.Processes {
		if proc.PID == p.PID && p.Started != 0 && proc.Started == p.Started {
			a.openProcess(proc, &p)
			return
		}
	}
	a.openProcess(monitor.Process{Identity: monitor.Identity{PID: p.PID, Started: p.Started}, Name: p.Process}, &p)
}

func (a *app) statusBar(c *ui.Context) {
	t := c.Theme()
	surface, _ := a.palette(c)
	ui.Row(c).Height(30).Shrink(0).Padding(0, 18).Gap(7).Background(surface).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		state := "就绪"
		if a.sampling || a.scanning {
			state = "正在更新…"
		} else if !a.snapshot.CPUReady {
			state = "等待第二次 CPU 采样…"
		}
		ui.Box(c).Size(5, 5).Radius(3).Background(green)
		ui.Text(c, state).FontSize(10).TextColor(t.TextMuted)
		ui.Spacer(c)
		warning := strings.Join(a.snapshot.Errors, "；")
		if a.portSnapshot.Err != "" {
			warning = a.portSnapshot.Err
		}
		if warning != "" {
			ui.Text(c, warning).FontSize(10).TextColor(t.Warning).SingleLine().MaxWidth(600).Tooltip(warning)
		} else if a.snapshot.Unreadable > 0 {
			ui.Textf(c, "%d 个进程的 CPU 信息不可读", a.snapshot.Unreadable).FontSize(10).TextColor(t.TextMuted)
		}
		if !a.portSnapshot.At.IsZero() {
			ui.Text(c, "端口更新 "+a.portSnapshot.At.Format("15:04:05")).FontSize(10).TextColor(t.TextMuted).Margin(0, 0, 0, 12)
		}
	})
}

func (a *app) settingsView(c *ui.Context) {
	ui.Row(c).Shrink(0).Gap(12).Children(func() {
		ui.Text(c, "设置").FontSize(25).Bold().Grow(1)
		label := "IP 标注簿"
		if a.ipNotes.showSettings {
			label = "返回常规设置"
		}
		ui.Button(c, label).OnClick(func() { a.ipNotes.showSettings = !a.ipNotes.showSettings })
	})
	if a.ipNotes.showSettings {
		a.ipNotesSettings(c)
		return
	}
	ui.Scroll(c).Grow(1).Gap(18).Children(func() {
		a.networkSettingsView(c)
		a.aiSettingsView(c)
		a.settingsCard(c, "刷新设置", func() {
			if ui.Checkbox(c, &a.config.AutoRefresh, "自动刷新监控数据").Changed() {
				a.persist()
				if a.config.AutoRefresh {
					a.requestRefresh()
				}
			}
			ui.Row(c).Gap(16).Children(func() {
				ui.Text(c, "CPU 与进程刷新间隔").Grow(1)
				if ui.Select(c, &a.interval, []string{"1 秒", "2 秒", "5 秒", "10 秒"}).Width(120).Label("刷新间隔").Changed() {
					fmt.Sscanf(a.interval, "%d 秒", &a.config.Interval)
					a.persist()
				}
			})
			ui.Text(c, "端口扫描至少间隔 5 秒；手动刷新会立即更新两类数据。").FontSize(11).TextColor(c.Theme().TextMuted)
		})
		a.settingsCard(c, "显示与启动", func() {
			if ui.Checkbox(c, &a.config.Tray, platformText("在菜单栏显示图标与整机 CPU 使用率", "在系统托盘显示图标")).Changed() {
				if a.win != nil {
					a.configureTray()
				}
				a.persist()
			}
			ui.Text(c, platformText("启用菜单栏图标时，关闭窗口会继续在后台运行；⌘Q 退出应用。", "启用托盘图标时，关闭窗口会继续在后台运行；从托盘菜单退出应用。")).FontSize(11).TextColor(c.Theme().TextMuted)
			if ui.Checkbox(c, &a.loginEnabled, "登录时自动启动").Changed() {
				if a.win != nil {
					if err := mygo.App.SetOpenAtLogin(a.loginEnabled); err != nil {
						a.loginEnabled = !a.loginEnabled
						a.notify("设置登录启动失败: " + err.Error())
					}
				}
			}
			if ui.Checkbox(c, &a.config.StartHidden, "登录启动时隐藏主窗口").Disabled(!a.config.Tray).Changed() {
				a.persist()
			}
		})
		a.settingsCard(c, "数据说明", func() {
			for _, text := range []string{"进程 CPU：100% 对应一个逻辑核心，多核进程可以超过 100%。", platformText("进程内存：驻留集 RSS，与活动监视器的内存占用口径可能不同。", "进程内存：工作集，与任务管理器的专用工作集口径可能不同。"), "TCP 端口：只列当前权限下可见的监听 socket；相同端点的重复文件描述符合并。", "端口计数：按 PID、协议、地址族、地址、端口组合计数。", "监控在本机完成；点击 AI 分析时，将所选进程的摘要发送至你配置的模型服务。"} {
				ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted)
			}
		})
	})
}

func (a *app) settingsCard(c *ui.Context, title string, children func()) {
	surface, _ := a.palette(c)
	ui.Column(c).Padding(20).Gap(15).Radius(11).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Children(func() { ui.Text(c, title).Bold().FontSize(14); ui.Divider(c); children() })
}

func (a *app) detailsView(c *ui.Context) {
	ui.Modal(c, &a.showDetail, func() {
		d := a.detail
		title := "进程详情"
		if a.detailPort != nil {
			title = "端口详情"
		}
		width := float32(620)
		if a.analysis.tab == "AI 分析" {
			width = 880
		}
		ui.Column(c).Width(width).MaxHeight(620).Gap(14).Children(func() {
			ui.Row(c).Gap(9).Children(func() {
				ui.Icon(c, cpuIcon).Size(20, 20).TextColor(blue)
				ui.Text(c, title).FontSize(20).Bold().Grow(1)
				closeLabel := "关闭"
				if a.connections.processOrigin != nil {
					closeLabel = "返回连接详情"
				}
				ui.Button(c, closeLabel).OnClick(a.closeProcessDetails)
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, placeholder(d.Name)).FontSize(17).Bold().Grow(1)
				ui.Textf(c, "PID  %d", d.PID).TextColor(c.Theme().TextMuted)
			})
			ui.Row(c).Gap(8).Children(func() {
				for _, tab := range []string{"进程信息", "AI 分析"} {
					b := ui.Button(c.Key(tab), tab)
					if a.analysis.tab == tab {
						b.TextColor(purple).Background(purple.Alpha(0.10))
					}
					b.OnClick(func() { a.analysis.tab = tab })
				}
			})
			ui.Divider(c)
			ui.Scroll(c).MaxHeight(400).Gap(12).Children(func() {
				if a.analysis.tab == "AI 分析" {
					a.analysisView(c)
					return
				}
				if p := a.detailPort; p != nil {
					a.detailRow(c, "端口", strconv.Itoa(p.Port))
					a.detailRow(c, "协议", p.Protocol+" / "+p.Family)
					a.detailRow(c, "监听地址", p.Address)
					a.detailRow(c, "扫描状态", "监听中（端口列表更新于 "+a.portSnapshot.At.Format("15:04:05")+"）")
				}
				a.detailRow(c, "CPU 使用率", cpuText(a.detailProcess))
				a.detailRow(c, "内存 RSS", memoryText(a.detailProcess))
				if a.detailLoading {
					ui.Text(c, "正在读取进程详情…").TextColor(c.Theme().TextMuted)
				} else {
					a.detailRow(c, "可执行路径", placeholder(d.Path))
					a.detailRow(c, "命令行", placeholder(d.Command))
					a.detailRow(c, "用户", placeholder(d.User))
					threads := "不可用"
					if d.Threads > 0 {
						threads = strconv.Itoa(int(d.Threads))
					}
					a.detailRow(c, "线程数", threads)
					if d.Started > 0 {
						a.detailRow(c, "启动时间", time.UnixMilli(d.Started).Local().Format("2006-01-02 15:04:05"))
					}
					for _, err := range d.Errors {
						ui.Text(c, err).FontSize(11).TextColor(c.Theme().Warning)
					}
				}
			})
			ui.Divider(c)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, platformText("在 Finder 中显示", "在资源管理器中显示")).Disabled(d.Path == "" || a.detailLoading).Clicked() {
					if a.win != nil {
						mygo.Shell.ShowItemInFolder(d.Path)
					}
				}
				if ui.Button(c, "复制路径").Disabled(d.Path == "" || a.detailLoading).Clicked() {
					a.copy(d.Path)
				}
				if ui.Button(c, "复制命令").Disabled(d.Command == "" || a.detailLoading).Clicked() {
					a.copy(d.Command)
				}
			})
			if a.detailPort != nil {
				ui.Text(c, "停止进程会影响它提供的全部服务，可能同时释放多个端口。").FontSize(11).TextColor(c.Theme().TextMuted)
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				p := monitor.Process{Identity: d.Identity, Name: d.Name}
				if ui.Button(c, "停止进程…").TextColor(c.Theme().Danger).Disabled(a.stopping || a.detailLoading || a.detailInspectError != "" || !canStopProcess(p) || (a.detailPort != nil && a.detailPort.Started == 0)).Clicked() {
					a.requestStopProcess(p)
				}
			})
		})
	})
}
func (a *app) detailRow(c *ui.Context, key, value string) {
	ui.Row(c).AlignItems(ui.Start).Gap(10).Children(func() {
		ui.Text(c, key).Width(90).Shrink(0).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Text(c, value).Grow(1).FontSize(12)
	})
}
func (a *app) copy(text string) {
	if a.win != nil {
		mygo.Clipboard.WriteText(text)
	}
	a.notify("已复制到剪贴板")
}
func (a *app) confirmView(c *ui.Context) {
	ui.Modal(c, &a.confirmStop, func() {
		ui.Column(c).Width(420).Gap(16).Children(func() {
			ui.Text(c, "确认停止此进程？").FontSize(19).Bold()
			ui.Textf(c, "%s（PID %d）", a.stopTarget.Name, a.stopTarget.PID).Bold()
			ui.Text(c, "进程中未保存的数据可能丢失，其占用的所有端口和服务都可能停止。")
			if runtime.GOOS == "windows" {
				ui.Text(c, "Windows 版将强制结束进程，无法让进程清理或保存数据。")
			} else {
				ui.Checkbox(c, &a.forceStop, "强制结束（SIGKILL，无法让进程清理数据）")
			}
			ui.Row(c).Justify(ui.End).Gap(10).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.confirmStop = false
				}
				if ui.PrimaryButton(c, "确认停止").Background(c.Theme().Danger).Clicked() {
					if a.win != nil {
						a.stopProcess()
					}
				}
			})
		})
	})
}
