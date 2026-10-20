package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/network"
)

var networkOrange = ui.Hex("#ff9b45")
var networkCyan = ui.Hex("#38c9ff")

func rateText(value float64, ready bool) string {
	if !ready {
		return "—"
	}
	if value < 1024 {
		return fmt.Sprintf("%.0f B/s", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KiB/s", value/1024)
	}
	return fmt.Sprintf("%.2f MiB/s", value/(1024*1024))
}
func networkBytes(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1<<20 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	if n < 1<<30 {
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.2f GiB", float64(n)/(1<<30))
}
func (a *app) networkOverview(c *ui.Context) {
	surface, _ := a.palette(c)
	light := a.network.light
	ui.Row(c).Shrink(0).Height(48).Padding(0, 16).Gap(12).Radius(9).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Children(func() {
		ui.Icon(c, networkIcon).Size(17, 17).TextColor(green)
		ui.Text(c, "网络").Bold().FontSize(12)
		ui.Text(c, "↓ "+rateText(light.Download, light.Ready)).TextColor(blue).FontSize(12)
		ui.Text(c, "↑ "+rateText(light.Upload, light.Ready)).TextColor(networkOrange).FontSize(12)
		label := "轻量网卡采样"
		if a.network.enabled {
			label = "详细分析已开启"
		}
		if light.Err != "" {
			label = light.Err
		}
		ui.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted).SingleLine().Grow(1)
		ui.Button(c, "查看网络").FontSize(11).OnClick(func() { a.page = "network" })
	})
}

func (a *app) networkSettingsView(c *ui.Context) {
	a.settingsCard(c, "网络与性能", func() {
		ui.Text(c, "日常仅每 2 秒读取物理网卡计数，不启动详细连接采集，也不查询外部服务。").FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Checkbox(c.Key("settings-network-analysis"), &a.network.enabled, "启用详细网络分析（本次运行）").Disabled(runtime.GOOS == "windows").OnChange(func() { a.setNetworkAnalysis(a.network.enabled) })
		ui.Text(c, platformText("开启后采集进程流量和 TCP / UDP 对端；关闭后终止采集并清空明细。重启应用默认关闭。", "Windows 版暂不支持逐进程／逐连接流量统计和连接地图；整机网速、网络连接和 IP 标注可用。")).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Checkbox(c.Key("settings-network-animation"), &a.config.NetworkAnimation, "显示光纤流动动画").Disabled(runtime.GOOS == "windows").OnChange(a.persist)
		ui.Text(c, "仅在网络页面可见且有传输时播放，隐藏或最小化窗口时暂停。关闭动画仍可查看静态地图和表格。").FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Checkbox(c.Key("settings-network-geo"), &a.config.NetworkGeo, "联网查询 IP 位置与组织（ipwho.is）").Disabled(runtime.GOOS == "windows").OnChange(a.changeNetworkGeo)
		ui.Text(c, "默认关闭。开启后仅在详细分析期间，向 ipwho.is 发送公网目标 IP；不发送进程名、本地地址或请求内容。每次分析最多查询 200 个 IP，结果为估算位置。").FontSize(11).TextColor(c.Theme().TextMuted)
	})
}

func (a *app) networkPage(c *ui.Context) {
	surface, _ := a.palette(c)
	n := &a.network
	_, height := c.Size()
	// Compact windows keep readable panel sizes and scroll the page. Taller
	// windows distribute their spare space between analysis and connections.
	minimumHeight := float32(960)
	for _, extra := range []bool{n.light.Err != "", n.err != "", n.snapshot.Dropped > 0, n.geoPaused} {
		if extra {
			minimumHeight += 28
		}
	}
	var page ui.Element
	if height < minimumHeight || !n.enabled {
		page = ui.Scroll(c.Key("network-page"))
	} else {
		page = ui.Column(c.Key("network-page"))
	}
	page.Grow(1).MinHeight(0).Gap(12).Children(func() {
		ui.Row(c).Height(84).Shrink(0).Gap(12).AlignItems(ui.Stretch).Children(func() {
			for i, card := range []struct {
				title, value, caption string
				color                 ui.Color
			}{
				{"当前下载", rateText(n.light.Download, n.light.Ready), "物理网卡 · " + strings.Join(n.light.Interfaces, ", "), blue},
				{"当前上传", rateText(n.light.Upload, n.light.Ready), "不重复叠加 VPN 虚拟网卡", networkOrange},
				{"本次监控流量", networkBytes(n.light.Received + n.light.Sent), "↓ " + networkBytes(n.light.Received) + "   ↑ " + networkBytes(n.light.Sent), green},
			} {
				ui.Column(c.Key(i)).Grow(1).Basis(0).MinWidth(0).Padding(12, 16).Gap(5).Radius(9).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Children(func() {
					ui.Text(c, card.title).FontSize(11).TextColor(c.Theme().TextMuted)
					ui.Text(c, card.value).FontSize(23).Bold().TextColor(card.color)
					ui.Text(c, card.caption).FontSize(9).TextColor(c.Theme().TextMuted).SingleLine()
				})
			}
		})
		if n.light.Err != "" {
			ui.Text(c, n.light.Err).FontSize(11).TextColor(c.Theme().Warning).Shrink(0)
		}
		ui.Row(c).Shrink(0).Gap(9).Children(func() {
			ui.SearchField(c.Key("network-search"), &n.query).Placeholder("搜索进程、PID、IP、端口或组织…").Label("搜索网络连接").Grow(1).MinWidth(120)
			ui.Select(c.Key("network-order"), &n.order, []string{"合计速度", "下载速度", "上传速度", "累计流量"}).Width(112).Label("流量排序")
			ui.Checkbox(c.Key("network-group"), &n.group, "按应用汇总").OnChange(func() { n.filter = ""; n.selectedProcess = ""; n.processSelected = -1 })
			label := "开始分析"
			if n.enabled {
				label = "停止分析"
			}
			ui.PrimaryButton(c, label).Disabled(runtime.GOOS == "windows").OnClick(func() { a.setNetworkAnalysis(!a.network.enabled) })
		})
		if n.err != "" {
			ui.Text(c, n.err+"。可重新开始分析；普通权限可能无法读取部分进程。").FontSize(11).TextColor(c.Theme().Warning).Shrink(0)
		}
		if !n.enabled {
			ui.Column(c).Height(230).Shrink(0).Center().Gap(12).Padding(24).Radius(12).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Children(func() {
				ui.Icon(c, networkIcon).Size(32, 32).TextColor(blue)
				ui.Text(c, "轻量监控正在运行").FontSize(19).Bold()
				ui.Text(c, platformText("需要定位谁在占用网络时，点击「开始分析」。", "Windows 版支持整机网速；查看进程与端点请打开「网络连接」。")).FontSize(13).TextColor(c.Theme().TextMuted)
				ui.Text(c, platformText("详细采集、IP 查询与光纤动画当前均未运行。", "逐进程／逐连接流量与连接地图暂不支持。")).FontSize(11).TextColor(c.Theme().TextMuted)
			})
			a.networkChart(c, 145)
			return
		}
		ui.Row(c.Key("network-analysis")).Grow(2).Basis(360).MinHeight(360).Shrink(0).Gap(12).AlignItems(ui.Stretch).Label("网络流量分析区").Children(func() {
			ui.Column(c).Width(390).Shrink(0).MinHeight(0).Radius(10).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Clip().Children(func() {
				ui.Row(c).Shrink(0).Padding(12, 14).Gap(6).Children(func() {
					ui.Text(c, "进程流量排行").Bold()
					ui.Spacer(c)
					ui.Textf(c, "%d 个进程", len(n.snapshot.Processes)).FontSize(10).TextColor(c.Theme().TextMuted)
				})
				a.networkProcessTable(c)
				ui.Text(c, "点击行筛选连接 · 累计从本次分析首个样本起算").FontSize(9).TextColor(c.Theme().TextMuted).Padding(9, 12).Shrink(0)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(10).Children(func() {
				a.networkMap(c)
				a.networkChart(c, 120)
			})
		})
		ui.Column(c.Key("network-connection-panel")).Grow(1).Basis(180).MinHeight(180).Shrink(0).Radius(10).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Clip().Children(func() {
			ui.Row(c).Shrink(0).Padding(9, 12).Gap(8).Children(func() {
				ui.Text(c, "网络连接").Bold()
				if n.filter != "" {
					ui.Button(c, "显示全部进程").FontSize(10).OnClick(func() { n.filter = ""; n.selectedProcess = ""; n.processSelected = -1 })
				}
				ui.Spacer(c)
				ui.Text(c, "双击行查看详情").FontSize(10).TextColor(c.Theme().TextMuted)
				ui.Button(c, "连接详情").FontSize(10).Disabled(n.selectedConnection == "").OnClick(func() {
					for _, row := range a.networkConnections() {
						if row.Key() == n.selectedConnection {
							n.detail = row
							n.detailOpen = true
							return
						}
					}
					a.notify("连接已结束或不在当前筛选中")
				})
			})
			a.networkConnectionTable(c)
		})
		stamp := "等待首个完整样本，约 2–4 秒…"
		if !n.snapshot.At.IsZero() {
			stamp = "采样 " + n.snapshot.At.Format("15:04:05") + " · 分析开始 " + n.started.Format("15:04:05") + " · 连接有约一轮采样的显示延迟"
		}
		ui.Text(c, stamp).FontSize(10).TextColor(c.Theme().TextMuted).Shrink(0)
		if n.snapshot.Dropped > 0 {
			ui.Textf(c, "为限制开销，本轮省略 %d 条记录", n.snapshot.Dropped).FontSize(10).TextColor(c.Theme().Warning).Shrink(0)
		}
		if n.geoPaused {
			ui.Text(c, "IP 查询服务已限流，本次分析暂停位置查询。进程与连接采集继续运行。").FontSize(10).TextColor(c.Theme().Warning).Shrink(0)
		}
		ui.Text(c, "进程统计包含本机与虚拟网络，不等于物理网卡总量；短连接可能遗漏。IP 位置为估算，连线为去向示意。").FontSize(10).TextColor(c.Theme().TextMuted).Shrink(0)
	})
}

func (a *app) networkProcessTable(c *ui.Context) {
	n := &a.network
	rows := a.networkProcesses()
	n.processList.Selected = &n.processSelected
	n.processList.KeepOffset = true
	n.processList.Key = func(i int) any { return rows[i].Key }
	n.processList.Label = func(i int) string { return rows[i].Name }
	table := ui.Table(c.Key("network-processes"), &n.processList, []ui.TableColumn{
		{Title: "应用 / 进程", MinWidth: 110}, {Title: "↓ 下载", Width: 83}, {Title: "↑ 上传", Width: 83}, {Title: "累计", Width: 80},
	}, len(rows), func(row, col int) {
		p := rows[row]
		switch col {
		case 0:
			ui.Text(c, p.Name).FontSize(11).SingleLine().Tooltip(fmt.Sprintf("%s · %d 个进程 · %d 条连接", p.Name, len(p.Processes), p.Connections))
		case 1:
			ui.Text(c, rateText(p.Traffic.Download, p.Traffic.Ready)).FontSize(10).TextColor(blue)
		case 2:
			ui.Text(c, rateText(p.Traffic.Upload, p.Traffic.Ready)).FontSize(10).TextColor(networkOrange)
		case 3:
			ui.Text(c, networkBytes(p.Traffic.Received+p.Traffic.Sent)).FontSize(10)
		}
	}).Grow(1).MinHeight(0).Label("进程网络流量排行")
	if table.Changed() && n.processSelected >= 0 && n.processSelected < len(rows) {
		n.selectedProcess = rows[n.processSelected].Key
		n.filter = n.selectedProcess
		c.Invalidate()
	}
	if len(rows) == 0 {
		table.Children(func() {
			ui.Text(c, "等待联网进程，或没有匹配项").Padding(18).FontSize(11).TextColor(c.Theme().TextMuted)
		})
	}
}
func (a *app) networkConnectionTable(c *ui.Context) {
	n := &a.network
	rows := a.networkConnections()
	n.connectionList.Selected = &n.connectionSelected
	n.connectionList.KeepOffset = true
	n.connectionList.Key = func(i int) any { return rows[i].Key() }
	n.connectionList.Label = func(i int) string { return rows[i].Process + " " + rows[i].Remote }
	table := ui.Table(c.Key("network-connections"), &n.connectionList, []ui.TableColumn{
		{Title: "进程", Width: 115}, {Title: "协议", Width: 54}, {Title: "本地地址", MinWidth: 160}, {Title: "远端地址", MinWidth: 160}, {Title: "↓ 下载", Width: 86}, {Title: "↑ 上传", Width: 86}, {Title: "网卡", Width: 55}, {Title: "IP 标注", MinWidth: 130},
	}, len(rows), func(row, col int) {
		x := rows[row]
		text := ""
		switch col {
		case 0:
			text = x.Process
		case 1:
			text = x.Protocol
		case 2:
			text = x.Local
		case 3:
			text = x.Remote
		case 4:
			text = rateText(x.Download, x.Ready)
		case 5:
			text = rateText(x.Upload, x.Ready)
		case 6:
			text = x.Interface
		case 7:
			note := a.ipNote(x.RemoteIP())
			if note.Name != "" {
				a.ipNoteBadge(c, note)
				return
			}
			text = "—"
		}
		color := c.Theme().Text
		if col == 4 {
			color = blue
		}
		if col == 5 {
			color = networkOrange
		}
		ui.Text(c, text).FontSize(10).TextColor(color).SingleLine().Tooltip(text)
	}).Grow(1).MinHeight(0).Label("网络连接列表")
	if table.Changed() && n.connectionSelected >= 0 && n.connectionSelected < len(rows) {
		n.selectedConnection = rows[n.connectionSelected].Key()
	}
	if table.Submitted() && n.connectionSelected >= 0 && n.connectionSelected < len(rows) {
		n.detail = rows[n.connectionSelected]
		n.detailOpen = true
	}
	if len(rows) == 0 {
		table.Children(func() {
			ui.Text(c, "暂无可见的有对端连接，或没有匹配项").Padding(16).FontSize(11).TextColor(c.Theme().TextMuted)
		})
	}
}

func (a *app) networkDetailView(c *ui.Context) {
	n := &a.network
	ui.Modal(c, &n.detailOpen, func() {
		d := n.detail
		// Keep identity fixed while refreshing values for the same observed flow.
		live := false
		for _, row := range n.snapshot.Connections {
			if row.Key() == d.Key() {
				d = row
				live = true
				break
			}
		}
		location := n.geo[d.RemoteIP()]
		ui.Column(c).Width(660).MaxHeight(610).Gap(14).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				ui.Icon(c, networkIcon).Size(20, 20).TextColor(blue)
				ui.Text(c, "网络连接详情").FontSize(20).Bold().Grow(1)
				ui.Button(c, "关闭连接详情").OnClick(func() { n.detailOpen = false })
			})
			ui.Text(c, d.Remote).FontSize(18).Bold()
			ui.Scroll(c).MaxHeight(430).Gap(12).Children(func() {
				a.detailRow(c, "所属进程", fmt.Sprintf("%s（PID %d）", d.Process, d.PID))
				a.detailRow(c, "本地端点", d.Local)
				a.detailRow(c, "远端端点", d.Remote)
				note := a.ipNote(d.RemoteIP())
				a.detailRow(c, "自定义 IP 标注", placeholder(note.Name))
				if note.Description != "" {
					a.detailRow(c, "标注说明", note.Description)
				}
				a.detailRow(c, "协议 / 网卡", d.Protocol+" / "+placeholder(d.Interface))
				state := placeholder(d.State)
				if !live {
					state = "连接已结束 / 暂不可见（保留所选快照）"
				}
				a.detailRow(c, "连接状态", state)
				a.detailRow(c, "网络范围", d.Scope())
				a.detailRow(c, "当前速度", "↓ "+rateText(d.Download, d.Ready)+"    ↑ "+rateText(d.Upload, d.Ready))
				a.detailRow(c, "观察期间累计", "↓ "+networkBytes(d.Received)+"    ↑ "+networkBytes(d.Sent))
				if location.Known {
					a.detailRow(c, "估算位置", location.Label())
					a.detailRow(c, "IP 注册组织", placeholder(location.Organization))
					a.detailRow(c, "ASN", fmt.Sprintf("AS%d", location.ASN))
				} else {
					text := "未开启联网位置查询"
					if a.config.NetworkGeo {
						text = placeholder(location.Err)
						if !network.PublicIP(d.RemoteIP()) {
							text = "本机或非公网 IP，不查询位置"
						}
					}
					a.detailRow(c, "位置与组织", text)
				}
				ui.Divider(c)
				ui.Text(c, "这里显示系统观测到的连接对端。代理 / VPN 可能改变可见去向；IP 注册组织、端口号和反向域名均不能证明具体网站或 HTTPS 请求内容。").FontSize(11).TextColor(c.Theme().TextMuted)
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "复制连接信息").OnClick(func() {
					a.copy(fmt.Sprintf("%s (PID %d)\n%s → %s\n%s / %s", d.Process, d.PID, d.Local, d.Remote, d.Protocol, d.Interface))
				})
				ui.Button(c, "标注远端 IP").Disabled(d.RemoteIP() == "").OnClick(func() { n.detailOpen = false; a.editIPNote(d.RemoteIP()) })
				ui.Button(c, "在地图中聚焦").OnClick(func() { n.detailOpen = false; n.selectedConnection = d.Key(); a.page = "network" })
			})
		})
	})
}

func sinceLabel(at time.Time) string {
	if at.IsZero() {
		return "—"
	}
	return at.Format("15:04:05")
}
