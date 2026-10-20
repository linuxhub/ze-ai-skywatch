package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/ipnotes"
)

func (a *app) connectionsPage(c *ui.Context) {
	n := &a.connections
	a.panel(c, func() {
		ui.Row(c).Padding(14, 16, 8).Gap(10).Children(func() {
			ui.SearchField(c.Key("socket-search"), &n.query).Placeholder("搜索进程、PID、IP、端口、网卡或标注…").Label("搜索连接").Grow(1).MinWidth(120)
			ui.Select(c.Key("socket-protocol"), &n.protocol, []string{"全部协议", "TCP", "UDP"}).Width(110).Label("连接协议筛选")
			ui.Select(c.Key("socket-mode"), &n.mode, []string{"全部连接", "监听与绑定", "有对端连接"}).Width(130).Label("连接视图筛选")
			label := "暂停刷新"
			if n.paused {
				label = "继续刷新"
			}
			ui.Button(c, label).OnClick(func() { n.paused = !n.paused; a.syncConnections() })
			ui.Button(c, "刷新连接").Disabled(n.scanning).OnClick(a.refreshConnections)
		})
		ui.Row(c).Padding(0, 16, 12).Gap(10).Children(func() {
			ui.Select(c.Key("socket-state"), &n.state, []string{"全部状态", "LISTEN", "ESTABLISHED", "其他"}).Width(150).Label("连接状态筛选")
			ui.Select(c.Key("socket-scope"), &n.scope, []string{"全部范围", "公网", "局域网", "本机回环", "通配 / 未知", "其他地址"}).Width(140).Label("连接范围筛选")
			ui.Select(c.Key("socket-annotation"), &n.annotation, []string{"全部标注", "已标注", "未标注"}).Width(120).Label("连接标注筛选")
			ui.Spacer(c)
			ui.Button(c, "管理 IP 标注").OnClick(func() { a.ipNotes.showSettings = true; a.page = "settings"; a.syncConnections() })
		})
		rows := a.filteredConnections()
		n.list.Selected, n.list.Sort, n.list.KeepOffset = &n.selected, &n.order, true
		n.list.Key = func(i int) any { return rows[i].Key() }
		n.list.Label = func(i int) string { return rows[i].Process + " " + rows[i].Remote.String() }
		// Scope the softer accent to the table so navigation and primary actions
		// keep their stronger emphasis. Rows built later while scrolling retain it.
		theme := c.Theme()
		tableTheme := *theme
		tableTheme.Accent, tableTheme.AccentText = ui.Hex("#eaf2ff"), theme.Text
		tableTheme.SurfaceHover = ui.Hex("#f4f7fc")
		downloadColor, uploadColor, statusColor := ui.Hex("#1463ce"), ui.Hex("#b95e0b"), ui.Hex("#13835b")
		if theme.Dark {
			tableTheme.Accent = ui.Hex("#263a55")
			tableTheme.SurfaceHover = ui.Hex("#252d3a")
			downloadColor, uploadColor, statusColor = ui.Hex("#82baff"), ui.Hex("#ffc078"), ui.Hex("#54d6a4")
		}
		c.SetTheme(&tableTheme)
		table := ui.Table(c.Key("socket-table"), &n.list, []ui.TableColumn{
			{Title: "进程", ID: "process", Width: 110, Sortable: true}, {Title: "PID", ID: "pid", Width: 56, Sortable: true},
			{Title: "协议", ID: "protocol", Width: 48, Sortable: true}, {Title: "本地 IP", ID: "local", MinWidth: 110, Sortable: true},
			{Title: "端口", ID: "localPort", Width: 66, Sortable: true}, {Title: "远端 IP", ID: "remote", MinWidth: 120, Sortable: true},
			{Title: "端口", ID: "remotePort", Width: 66, Sortable: true}, {Title: "状态", ID: "state", Width: 102, Sortable: true},
			{Title: "↓ 下载", ID: "download", Width: 86, Sortable: true}, {Title: "↑ 上传", ID: "upload", Width: 86, Sortable: true},
			{Title: "网卡", ID: "interface", Width: 64, Sortable: true},
			{Title: "IP 标注", ID: "note", MinWidth: 145, Sortable: true},
		}, len(rows), func(row, col int) {
			x := rows[row]
			text := ""
			switch col {
			case 0:
				text = x.Process
			case 1:
				text = fmt.Sprint(x.PID)
			case 2:
				text = x.Protocol
			case 3:
				text = x.Local.IP
			case 4:
				text = x.Local.Port
			case 5:
				text = x.Remote.IP
			case 6:
				text = x.Remote.Port
			case 7:
				text = x.Status()
			case 8:
				text = rateText(x.Download, x.Ready)
			case 9:
				text = rateText(x.Upload, x.Ready)
			case 10:
				text = x.Interface
			case 11:
				note := a.ipNote(x.Remote.IP)
				if note.Name != "" {
					a.ipNoteBadge(c, note)
					return
				}
			}
			if text == "" {
				text = "—"
			}
			color := c.Theme().Text
			if col == 8 {
				color = downloadColor
			}
			if col == 9 {
				color = uploadColor
			}
			if col == 7 && (x.State == "ESTABLISHED" || x.State == "LISTEN") {
				color = statusColor
			}
			ui.Text(c, text).FontSize(11).TextColor(color).SingleLine().Tooltip(text)
		}).Grow(1).MinHeight(0).Label("独立网络连接列表")
		c.SetTheme(theme)
		_, bg := a.palette(c)
		c.Root().Background(bg)
		if table.Changed() {
			if n.selected >= 0 && n.selected < len(rows) {
				n.selectedKey = rows[n.selected].Key()
			} else {
				n.selectedKey = ""
			}
			c.Invalidate()
		}
		if table.Submitted() && n.selected >= 0 && n.selected < len(rows) {
			n.detail = rows[n.selected]
			n.detailOpen = true
		}
		if len(rows) == 0 {
			table.Children(func() {
				ui.Text(c, "暂无可见连接，或没有匹配项").FontSize(12).TextColor(c.Theme().TextMuted).Padding(20)
			})
		}
		ui.Row(c).Padding(12, 16).Gap(10).Children(func() {
			stamp := "尚未采集"
			if !n.snapshot.At.IsZero() {
				stamp = "采集于 " + n.snapshot.At.Format("15:04:05")
			}
			if n.scanning {
				stamp += " · 正在采集…"
			}
			ui.Textf(c, "%d / %d 条 · %s", len(rows), len(n.snapshot.Rows), stamp).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
			selected, ok := a.selectedSocket()
			ui.Button(c, "标注远端 IP").Disabled(!ok || ipnotes.NormalizeIP(selected.Remote.IP) == "").OnClick(func() { a.editIPNote(selected.Remote.IP) })
			ui.Button(c, "查看连接详情").Disabled(!ok).OnClick(func() { n.detail = selected; n.detailOpen = true })
		})
	})
	if n.snapshot.Err != "" {
		ui.Text(c, n.snapshot.Err+"（保留上次成功结果）").FontSize(11).TextColor(c.Theme().Warning)
	}
	if n.snapshot.Warning != "" {
		ui.Text(c, n.snapshot.Warning).FontSize(11).TextColor(c.Theme().Warning).SingleLine().Tooltip(n.snapshot.Warning)
	}
	if n.trafficErr != "" {
		ui.Text(c, n.trafficErr).FontSize(11).TextColor(c.Theme().Warning).SingleLine().Tooltip(n.trafficErr)
	}
	ui.Text(c, "速率首次采样或不可用时显示 —；通配绑定无固定网卡。本地 / 远端表示连接两端；普通权限下部分连接不可见。").FontSize(10).TextColor(c.Theme().TextMuted)
}

func (a *app) socketDetailView(c *ui.Context) {
	n := &a.connections
	ui.Modal(c.Key("socket-detail"), &n.detailOpen, func() {
		d := n.detail
		live := false
		for _, row := range n.snapshot.Rows {
			if row.Key() == d.Key() {
				d = row
				live = true
				break
			}
		}
		note := a.ipNote(d.Remote.IP)
		ui.Column(c).Width(640).MaxHeight(610).Gap(14).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				ui.Text(c, "连接端点详情").FontSize(20).Bold().Grow(1)
				ui.Button(c, "关闭端点详情").OnClick(func() { n.detailOpen = false })
			})
			ui.Scroll(c).MaxHeight(430).Gap(12).Children(func() {
				a.detailRow(c, "所属进程", fmt.Sprintf("%s（PID %d）", d.Process, d.PID))
				a.detailRow(c, "协议 / 地址族", d.Protocol+" / "+d.Family)
				a.detailRow(c, "本地 IP / 端口", d.Local.String())
				a.detailRow(c, "远端 IP / 端口", d.Remote.String())
				a.detailRow(c, "连接状态", d.Status())
				a.detailRow(c, "下载速率", rateText(d.Download, d.Ready))
				a.detailRow(c, "上传速率", rateText(d.Upload, d.Ready))
				a.detailRow(c, "网卡", placeholder(d.Interface))
				a.detailRow(c, "网络范围", d.Scope())
				a.detailRow(c, "采集时间", sinceLabel(n.snapshot.At))
				if !live {
					ui.Text(c, "此连接已结束或暂不可见，正在显示所选快照。").TextColor(c.Theme().Warning)
				}
				if d.Started <= 0 {
					ui.Text(c, "尚未确认所属进程身份，暂不能查看或结束进程。请刷新连接后重试。").FontSize(11).TextColor(c.Theme().TextMuted)
				}
				if n.paused {
					ui.Text(c, "自动刷新已暂停，状态以采集时间为准。").TextColor(c.Theme().TextMuted)
				}
				if n.snapshot.Err != "" {
					ui.Text(c, n.snapshot.Err).TextColor(c.Theme().Warning)
				}
				a.detailRow(c, "自定义 IP 标注", placeholder(note.Name))
				if note.Group != "" {
					a.detailRow(c, "标注分组", note.Group)
				}
				if note.Description != "" {
					a.detailRow(c, "标注说明", note.Description)
				}
				ui.Text(c, "这里显示系统可见的 socket。代理 / VPN 可能改变远端地址；IP 标注是你维护的业务说明。").FontSize(11).TextColor(c.Theme().TextMuted)
			})
			ui.Row(c).Gap(10).Children(func() {
				ui.Button(c, "复制端点信息").OnClick(func() { a.copy(socketText(d) + "\nIP 标注：" + note.Name) })
				label := "标注此 IP"
				if note.Name != "" {
					label = "编辑此 IP 标注"
				}
				ui.PrimaryButton(c, label).Disabled(ipnotes.NormalizeIP(d.Remote.IP) == "").OnClick(func() { n.detailOpen = false; a.editIPNote(d.Remote.IP) })
				ui.Spacer(c)
				ui.Button(c, "查看进程").Disabled(d.PID <= 0 || d.Started <= 0).OnClick(func() { a.openSocketProcess(d) })
				p := a.socketProcess(d)
				ui.Button(c, "结束此进程…").TextColor(c.Theme().Danger).Disabled(a.stopping || !canStopProcess(p)).OnClick(func() { a.requestStopProcess(p) })
			})
		})
	})
}
