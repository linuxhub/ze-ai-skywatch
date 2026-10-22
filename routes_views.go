package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) routesCard(c *ui.Context) {
	state := &a.netconfig
	table := state.snapshot.Routing
	services := c.Services()
	routeColor := purple
	if c.Theme().Dark {
		routeColor = ui.Hex("#bca3ff")
	}
	surface, _ := a.palette(c)
	ui.Column(c.Key("routes-card")).Grow(1).MinHeight(0).Padding(20).Gap(15).Radius(11).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Label("路由与 VPN 分流卡片").Children(func() {
		ui.Text(c, "路由与 VPN 分流").Bold().FontSize(14).Shrink(0)
		ui.Divider(c).Shrink(0)
		if table.Err != "" {
			ui.Text(c, table.Err+"（以下为上次路由，暂停出口推断）").FontSize(11).TextColor(c.Theme().Warning).Shrink(0)
		}
		count := 0
		interfaces := make(map[string]bool)
		for _, rule := range table.Rows {
			if rule.TunnelPolicy() {
				count++
				interfaces[rule.Interface] = true
			}
		}
		ui.Textf(c, "%d 条通用路由经 VPN／隧道接口 · %d 个相关接口", count, len(interfaces)).Bold().FontSize(13).TextColor(routeColor).Shrink(0)
		ui.Row(c).Gap(10).Shrink(0).Children(func() {
			ui.Select(c.Key("route-filter"), &state.routeFilter, []string{"VPN／隧道路由", "全部路由", "IPv4 路由", "IPv6 路由"}).Width(180).Label("路由筛选")
			ui.Textf(c, "IPv4 默认：%s · 网关 %s", networkValue(state.snapshot.Default4.Interface), networkValue(state.snapshot.Default4.Gateway)).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
			ui.Button(c, "复制全部路由").Disabled(table.At.IsZero()).OnClick(func() { services.WriteClipboard(table.Text()); a.notify("已复制路由与分流规则") })
		})
		rows := a.filteredRoutes()
		state.routeList.Key = func(index int) any { return rows[index].Key() }
		state.routeList.Label = func(index int) string { return rows[index].Destination + " " + rows[index].Interface }
		ui.Table(c.Key("route-table"), &state.routeList, []ui.TableColumn{
			{Title: "目标网段", MinWidth: 170}, {Title: "网关／下一跳", MinWidth: 170}, {Title: "出口网卡", MinWidth: 100}, {Title: "走向", Width: 110}, {Title: "范围／总跃点", Width: 140},
		}, len(rows), func(row, column int) {
			rule := rows[row]
			text := ""
			switch column {
			case 0:
				text = rule.Destination
			case 1:
				text = rule.NextHop()
			case 2:
				text = rule.Interface
			case 3:
				text = rule.Path()
			case 4:
				text = "通用"
				if rule.Scoped {
					text = "仅限接口"
				}
				if rule.MetricKnown {
					text += " · " + rule.MetricText()
				}
			}
			color := c.Theme().Text
			if rule.TunnelPolicy() {
				color = routeColor
			}
			ui.Text(c, text).FontSize(11).SingleLine().Tooltip(text).TextColor(color)
		}).Grow(1).Basis(0).MinHeight(0).Label("系统路由与分流列表")
		ui.Column(c).Gap(10).Shrink(0).Children(func() {
			if len(rows) == 0 {
				ui.Text(c, "当前筛选没有路由；可切换“全部路由”查看默认出口与其他规则。").FontSize(11).TextColor(c.Theme().TextMuted)
			}
			ui.Row(c).Gap(10).Children(func() {
				ui.Text(c, "目标 IP").Width(70).Shrink(0)
				ui.SearchField(c.Key("route-target"), &state.routeQuery).Placeholder("例如 192.168.110.1，或 IPv6 地址").Label("查询目标 IP 路由").Grow(1).MinWidth(0)
			})
			if strings.TrimSpace(state.routeQuery) != "" {
				match := a.matchRoute(state.routeQuery)
				a.netconfigField(c, "出口推断", match.Text())
			}
			stamp := "等待路由采集"
			if !table.At.IsZero() {
				stamp = fmt.Sprintf("路由采集 %s · 当前筛选 %d / %d 条", table.At.Format("15:04:05"), len(rows), len(table.Rows))
			}
			ui.Text(c, stamp).FontSize(10).TextColor(c.Theme().TextMuted)
			if table.Truncated {
				ui.Text(c, "路由超过显示上限，暂停出口推断。").FontSize(11).TextColor(c.Theme().Warning)
			}
			ui.Text(c, "目标按最长网段匹配；Windows 同网段比较路由与接口总跃点。macOS 接口限定规则单独标识，不作为通用出口。").FontSize(11).TextColor(c.Theme().TextMuted)
			ui.Text(c, "这里展示系统路由的出口推断，不发送探测请求。应用绑定接口、既有连接及应用自设代理可能使用不同路径；接口名不能确认 VPN 客户端身份。").FontSize(11).TextColor(c.Theme().TextMuted)
		})
	})
}
