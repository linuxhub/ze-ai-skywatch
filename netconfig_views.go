package main

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/netconfig"
)

func networkValue(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func networkRoute(route netconfig.Route, adapters []netconfig.Adapter, ipv6 bool) string {
	if route.Interface == "" {
		return "未获取／未配置"
	}
	label := route.Interface
	for _, adapter := range adapters {
		if adapter.Name != route.Interface {
			continue
		}
		for _, address := range adapter.Addresses {
			ip, err := netip.ParseAddr(address.IP)
			if err == nil && ip.Is6() == ipv6 {
				label += " · " + address.IP
				break
			}
		}
	}
	return label + "\n网关 " + networkValue(route.Gateway)
}

func (a *app) netconfigPage(c *ui.Context) {
	state := &a.netconfig
	services := c.Services()
	ui.Row(c).Gap(10).Shrink(0).Children(func() {
		ui.Button(c, "刷新配置").Disabled(state.scanning).OnClick(func() { send(state.refresh) })
		ui.Button(c, "复制全部网络信息").Disabled(state.snapshot.At.IsZero()).OnClick(func() { services.WriteClipboard(state.snapshot.Text()); a.notify("已复制网络信息") })
		if state.tab == 0 {
			ui.Checkbox(c.Key("netconfig-show-all"), &state.showAll, "显示未连接与虚拟网卡").OnChange(func() {
				for _, adapter := range a.netconfigAdapters() {
					if adapter.Name == state.selected {
						return
					}
				}
				state.selected = ""
			})
		}
		ui.Spacer(c)
		stamp := "等待采集"
		if !state.snapshot.At.IsZero() {
			stamp = "采集 " + state.snapshot.At.Format("15:04:05")
		}
		if state.scanning {
			stamp += " · 正在刷新…"
		}
		ui.Text(c, stamp).FontSize(11).TextColor(c.Theme().TextMuted)
	})
	ui.Tabs(c.Key("netconfig-tabs"), &state.tab, "网卡与代理", "路由与分流").Shrink(0)
	// Routing is a workspace with a scrolling table. Give it the remaining
	// height instead of nesting a fixed-height table inside a scrolling page.
	if state.tab == 1 {
		ui.Column(c.Key("netconfig-routing")).Grow(1).MinHeight(0).Gap(16).Children(func() {
			if state.snapshot.Err != "" {
				ui.Text(c, "采集失败，保留上次结果："+state.snapshot.Err).TextColor(c.Theme().Danger).FontSize(12).Shrink(0)
			}
			a.routesCard(c)
		})
		return
	}
	ui.Scroll(c.Key(fmt.Sprintf("netconfig-scroll-%d", state.tab))).Grow(1).MinHeight(0).Gap(16).Children(func() {
		if state.snapshot.Err != "" {
			ui.Text(c, "采集失败，保留上次结果："+state.snapshot.Err).TextColor(c.Theme().Danger).FontSize(12)
		}
		a.settingsCard(c, "当前默认出口", func() {
			ui.Row(c).Gap(20).AlignItems(ui.Start).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(12).Children(func() {
					a.netconfigField(c, "IPv4 出口", networkRoute(state.snapshot.Default4, state.snapshot.Adapters, false))
					if state.snapshot.Default6.Interface != "" {
						a.netconfigField(c, "IPv6 出口", networkRoute(state.snapshot.Default6, state.snapshot.Adapters, true))
					}
				})
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(12).Children(func() {
					a.netconfigField(c, "系统 DNS", networkValue(strings.Join(state.snapshot.DNS, "、")))
					a.netconfigField(c, "系统代理", state.snapshot.ProxyStatus())
				})
			})
		})
		ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
			surface, _ := a.palette(c)
			ui.Column(c.Key("netconfig-adapters")).Width(250).Shrink(0).Padding(16).Gap(8).Radius(10).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Children(func() {
				ui.Text(c, "网卡列表").Bold().FontSize(14).Padding(0, 0, 6)
				rows := a.netconfigAdapters()
				if len(rows) == 0 {
					ui.Text(c, "暂无已连接的物理网卡，可勾选显示全部网卡。").FontSize(12).TextColor(c.Theme().TextMuted)
				}
				for _, adapter := range rows {
					ui.Button(c.Key(adapter.Name), adapter.Service+" · "+adapter.Name).Label("选择网卡 " + adapter.Name).FillWidth().FontSize(11).OnClick(func() { state.selected = adapter.Name })
					color := c.Theme().TextMuted
					if adapter.Name == state.selected {
						color = blue
					}
					status := adapter.Status
					if adapter.Name == state.snapshot.Default4.Interface {
						status += " · IPv4 默认"
					}
					if adapter.Name == state.snapshot.Default6.Interface {
						status += " · IPv6 默认"
					}
					ui.Text(c, status).TextColor(color).FontSize(10).Padding(0, 0, 8)
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				var selected *netconfig.Adapter
				rows := a.netconfigAdapters()
				for index := range rows {
					if rows[index].Name == state.selected {
						selected = &rows[index]
						break
					}
				}
				if selected == nil && len(rows) > 0 {
					selected = &rows[0]
				}
				if selected == nil {
					a.settingsCard(c, "网卡详情", func() { ui.Text(c, "选择网卡查看地址与配置。").TextColor(c.Theme().TextMuted) })
					return
				}
				adapter := *selected
				a.settingsCard(c, adapter.Service+" · "+adapter.Name, func() {
					a.adapterConfiguration(c, adapter)
				})
			})
		})
		a.settingsCard(c, "代理与隧道", func() {
			ui.Text(c, state.snapshot.ProxyScope).FontSize(11).TextColor(c.Theme().TextMuted)
			canCheck := false
			for _, proxy := range state.snapshot.Proxies {
				text := proxy.Status()
				if endpoint := proxy.Endpoint(); endpoint != "" {
					text += " · " + endpoint
				}
				if check := state.checks[proxy.Kind]; check != "" {
					text += " · " + check
				}
				a.netconfigField(c, proxy.Kind, text)
				canCheck = canCheck || (proxy.Known && proxy.Enabled && proxy.Server != "" && proxy.Port > 0)
			}
			a.netconfigField(c, "绕过列表", networkValue(strings.Join(state.snapshot.Exceptions, "、")))
			var tunnels []string
			for _, adapter := range state.snapshot.Adapters {
				if adapter.Virtual && adapter.Up && (strings.Contains(adapter.Kind, "VPN") || strings.Contains(adapter.Kind, "隧道") || strings.Contains(strings.ToLower(adapter.Kind), "tunnel") || strings.Contains(strings.ToLower(adapter.Kind), "vpn")) {
					tunnels = append(tunnels, adapter.Service+" ("+adapter.Name+")")
				}
			}
			a.netconfigField(c, "活动隧道接口", networkValue(strings.Join(tunnels, "、")))
			ui.Button(c, "检测代理端口").Disabled(!canCheck || state.checking || state.scanning).OnClick(a.checkNetconfigProxies)
			ui.Text(c, "点击后仅测试已启用固定代理的 TCP 端口；PAC 不执行。接口存在或端口可连接，不代表实际流量经过代理。").FontSize(11).TextColor(c.Theme().TextMuted)
		})
		for _, warning := range state.snapshot.Warnings {
			ui.Text(c, warning).FontSize(11).TextColor(c.Theme().Warning)
		}
		ui.Text(c, "DNS 可能按接口或域名分别解析；此处系统 DNS 展示默认解析器。PAC 地址的凭据、查询参数已隐藏。仅查看配置，不修改系统设置。").FontSize(11).TextColor(c.Theme().TextMuted)
	})
}

func (a *app) netconfigField(c *ui.Context, label, value string, identity ...string) {
	services := c.Services()
	key := label
	if len(identity) > 0 {
		key = identity[0]
	}
	ui.Row(c.Key(key)).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Text(c, label).Width(92).Shrink(0).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Text(c, value).Grow(1).MinWidth(0).FontSize(12)
		ui.Button(c, "复制").Label("复制" + label).FontSize(10).Shrink(0).OnClick(func() { services.WriteClipboard(value); a.notify("已复制" + label) })
	})
}
