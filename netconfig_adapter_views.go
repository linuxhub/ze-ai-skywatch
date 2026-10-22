package main

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/netconfig"
)

func adapterAddresses(adapter netconfig.Adapter, ipv6 bool) []netconfig.Address {
	var addresses []netconfig.Address
	for _, address := range adapter.Addresses {
		if ip, err := netip.ParseAddr(address.IP); err == nil && ip.Is6() == ipv6 {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

func adapterIPv6Visible(adapter netconfig.Adapter) bool {
	if adapter.IPv6Method == "已关闭" {
		return false
	}
	return len(adapterAddresses(adapter, true)) > 0 || len(adapter.Gateway6) > 0 || (adapter.IPv6Method != "" && adapter.IPv6Method != netconfig.Unknown)
}

func (a *app) adapterConfiguration(c *ui.Context, adapter netconfig.Adapter) {
	ui.Row(c).Gap(10).Children(func() {
		ui.Text(c, adapter.Kind).Grow(1).FontSize(11).TextColor(c.Theme().TextMuted)
		color := c.Theme().TextMuted
		if adapter.Up {
			color = green
		}
		ui.Text(c, adapter.Status).FontSize(11).TextColor(color)
	})
	showIPv6 := adapterIPv6Visible(adapter)
	if len(adapterAddresses(adapter, false)) > 0 || len(adapter.Gateway4) > 0 || (adapter.IPv4Method != "" && adapter.IPv4Method != netconfig.Unknown) || !showIPv6 {
		a.adapterAddressGroup(c, adapter, false)
	}
	if showIPv6 {
		a.adapterAddressGroup(c, adapter, true)
	}
	a.adapterGroup(c, "dns", func() {
		a.adapterField(c, "DNS 获取", networkValue(adapter.DNSMode))
		a.adapterField(c, "DNS 服务器", networkValue(strings.Join(adapter.DNS, "、")))
	})
	a.adapterGroup(c, "hardware", func() {
		if adapter.MAC != "" {
			a.adapterField(c, "MAC 地址", adapter.MAC)
		}
		if adapter.MTU > 0 {
			a.adapterField(c, "MTU", fmt.Sprint(adapter.MTU))
		}
	})
}

func (a *app) adapterAddressGroup(c *ui.Context, adapter netconfig.Adapter, ipv6 bool) {
	family, method, gateways := "IPv4", adapter.IPv4Method, adapter.Gateway4
	if ipv6 {
		family, method, gateways = "IPv6", adapter.IPv6Method, adapter.Gateway6
	}
	addresses := adapterAddresses(adapter, ipv6)
	a.adapterGroup(c, family, func() {
		if method == "" {
			method = netconfig.Unknown
		}
		a.adapterField(c, "配置 "+family, method)
		if len(addresses) == 0 {
			a.adapterField(c, "IP 地址", "未分配")
		}
		for index, address := range addresses {
			suffix := ""
			if index > 0 {
				suffix = fmt.Sprintf(" %d", index+1)
			}
			label := "IP 地址"
			if ipv6 {
				label = "IPv6 地址"
			}
			a.adapterField(c, label+suffix, address.IP, "address-"+address.IP)
			if ipv6 {
				prefix := netconfig.Unknown
				if address.Prefix >= 0 && address.Prefix <= 128 {
					prefix = fmt.Sprint(address.Prefix)
				}
				a.adapterField(c, "前缀长度"+suffix, prefix, "prefix-"+address.IP)
			} else {
				mask := address.SubnetMask()
				if mask == "" {
					mask = netconfig.Unknown
				}
				a.adapterField(c, "子网掩码"+suffix, mask, "mask-"+address.IP)
			}
			if address.Method != "" {
				a.adapterField(c, "地址来源"+suffix, address.Method, "origin-"+address.IP)
			}
		}
		if len(gateways) > 0 {
			a.adapterField(c, "网关地址", strings.Join(gateways, "、"))
		}
	})
}

func (a *app) adapterGroup(c *ui.Context, key string, children func()) {
	background := ui.Hex("#f5f6f8")
	if c.Theme().Dark {
		background = ui.Hex("#282f3a")
	}
	ui.Column(c.Key(key)).FillWidth().Gap(0).Radius(8).Background(background).Clip().Children(children)
}

func (a *app) adapterField(c *ui.Context, label, value string, identity ...string) {
	key := label
	if len(identity) > 0 {
		key = identity[0]
	}
	services := c.Services()
	ui.Row(c.Key(key)).Gap(12).Padding(7, 12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border.Alpha(0.45)).Children(func() {
		ui.Text(c, label).Width(100).Shrink(0).FontSize(12)
		ui.Text(c, value).Grow(1).MinWidth(0).FontSize(12).TextAlign(ui.End).TextColor(c.Theme().TextMuted).Selectable()
		button := ui.ButtonBase(c.Key("copy")).Size(24, 24).Shrink(0).Radius(4).Center().Label("复制" + label).Tooltip("复制" + label)
		if button.Hovered() {
			button.Background(c.Theme().SurfaceHover)
		}
		button.Children(func() { ui.Icon(c, copyIcon).Size(13, 13).TextColor(c.Theme().TextMuted) })
		button.OnClick(func() { services.WriteClipboard(value); a.notify("已复制" + label) })
	})
}
