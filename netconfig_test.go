package main

import (
	"context"
	"image/png"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
	"github.com/linuxhub/ze-ai-skywatch/internal/netconfig"
)

func fixtureNetconfig() netconfig.Snapshot {
	return netconfig.Snapshot{
		At: time.Now(), Default4: netconfig.Route{Interface: "en8", Gateway: "172.18.13.236"}, DNS: []string{"172.18.13.236"},
		ProxyScope: "当前系统代理；应用自设代理与隧道转发单独判断",
		Adapters: []netconfig.Adapter{
			{Index: 10, Name: "en8", Service: "USB LAN", Kind: "USB 10/100/1000 LAN", Up: true, Status: "已连接", MAC: "00:e0:4c:68:02:87", MTU: 1500, IPv4Method: "手动固定 IP", IPv6Method: "已关闭", Addresses: []netconfig.Address{{IP: "172.18.13.213", Prefix: 23}}, Gateway4: []string{"172.18.13.236"}, DNS: []string{"172.18.13.236"}, DNSMode: "手动配置"},
			{Index: 12, Name: "en0", Service: "Wi-Fi", Kind: "Wi-Fi", Up: true, Status: "已连接", MTU: 1500, IPv4Method: "DHCP 自动获取", IPv6Method: "自动配置（SLAAC／DHCPv6）", Addresses: []netconfig.Address{{IP: "192.168.50.62", Prefix: 24}, {IP: "fe80::1234:5678:abcd:ef01", Prefix: 64}}, Gateway4: []string{"192.168.50.1"}, DNS: []string{"1.1.1.1"}, DNSMode: "手动配置"},
			{Index: 16, Name: "utun6", Service: "utun6", Kind: "隧道／VPN 接口", Up: true, Virtual: true, Status: "已连接", MTU: 1380, IPv4Method: netconfig.Unknown, IPv6Method: netconfig.Unknown},
		},
		Proxies: []netconfig.Proxy{{Kind: "HTTP", Known: true, Enabled: true, Server: "127.0.0.1", Port: 7890}, {Kind: "HTTPS", Known: true}, {Kind: "SOCKS", Known: true}, {Kind: "PAC", Known: true}, {Kind: "自动发现", Known: true}},
	}
}

func TestNetconfigNavigationSelectionAndCopy(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.netconfig.snapshot = fixtureNetconfig()
	tester := ui.NewTester(a.view, 1360, 850)
	tester.SetLocale("zh-Hans")
	if err := tester.Click("网络配置"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("172.18.13.213") || !tester.HasText("255.255.254.0") {
		t.Fatal("default adapter details missing")
	}
	if err := tester.Click("选择网卡 en0"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("DHCP 自动获取") || !tester.HasText("手动配置") || tester.HasText("255.255.254.0") {
		t.Fatal("selected adapter details or independent DNS mode incorrect")
	}
	if err := tester.Click("复制配置 IPv4"); err != nil {
		t.Fatal(err)
	}
	if tester.Clipboard() != "DHCP 自动获取" {
		t.Fatal(tester.Clipboard())
	}
	if err := tester.Click("显示未连接与虚拟网卡"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("选择网卡 utun6"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText(netconfig.Unknown) {
		t.Fatal("virtual adapter unknown mode hidden")
	}
	if err := tester.Click("显示未连接与虚拟网卡"); err != nil {
		t.Fatal(err)
	}
	if tester.HasText("选择网卡 utun6") {
		t.Fatal("virtual adapter remained in filtered list")
	}
	if err := tester.Click("复制全部网络信息"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tester.Clipboard(), "192.168.50.62/24") || !strings.Contains(tester.Clipboard(), "HTTP：已启用 127.0.0.1:7890") {
		t.Fatal("copy omitted adapters or proxy details")
	}
	tester.Scroll(1000, 650, 0, 3000)
	if err := tester.Click("复制HTTP"); err != nil {
		t.Fatal(err)
	}
	if tester.Clipboard() != "已启用 · 127.0.0.1:7890" {
		t.Fatal("proxy details were not reachable or copied")
	}
	for _, page := range []string{"网络连接", "关于", "网络配置", "CPU 进程", "网络配置"} {
		if err := tester.Click(page); err != nil {
			t.Fatal(err)
		}
		tester.Frame()
	}
}

func TestNetconfigRejectsObsoleteResultsAndRetainsSuccessOnFailure(t *testing.T) {
	a := newApp()
	defer a.cancel()
	a.netconfig.generation = 3
	a.netconfig.snapshot = fixtureNetconfig()
	a.netconfig.scanning = true
	a.publishNetconfig(2, netconfig.Snapshot{})
	if !a.netconfig.scanning || len(a.netconfig.snapshot.Adapters) != 3 {
		t.Fatal("obsolete result replaced snapshot")
	}
	a.publishNetconfig(3, netconfig.Snapshot{Err: "权限不足"})
	if a.netconfig.scanning || a.netconfig.snapshot.Err != "权限不足" || len(a.netconfig.snapshot.Adapters) != 3 {
		t.Fatal("error discarded last successful data")
	}
	a.netconfig.selected = "removed-interface"
	a.publishNetconfig(3, fixtureNetconfig())
	if a.netconfig.snapshot.Err != "" || a.netconfig.selected != "en8" {
		t.Fatal("success did not reset error or selection")
	}
	a.netconfig.snapshot.Default4 = netconfig.Route{Interface: "utun6"}
	if len(a.netconfigAdapters()) != 3 {
		t.Fatal("virtual default route must remain visible")
	}
}

func TestNetconfigLeavingPageCancelsCollectorsAndChecks(t *testing.T) {
	a := newApp()
	defer a.cancel()
	collectCtx, stopCollect := context.WithCancel(a.ctx)
	checkCtx, stopCheck := context.WithCancel(a.ctx)
	a.netconfig.running, a.netconfig.scanning, a.netconfig.checking = true, true, true
	a.netconfig.cancel, a.netconfig.checkCancel = stopCollect, stopCheck
	a.netconfig.generation, a.netconfig.checkGeneration = 5, 8
	a.syncNetconfig()
	if collectCtx.Err() == nil || checkCtx.Err() == nil || a.netconfig.running || a.netconfig.scanning || a.netconfig.checking {
		t.Fatal("leaving page did not cancel collection and proxy checks")
	}
	if a.netconfig.generation != 6 || a.netconfig.checkGeneration != 9 {
		t.Fatal("canceled jobs can still publish")
	}
}

func TestNetconfigSeparatesIPAndMaskAndHidesDisabledIPv6(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page = "netconfig"
	a.netconfig.snapshot = fixtureNetconfig()
	tester := ui.NewTester(a.view, 1360, 850)
	tester.SetLocale("zh-Hans")
	for _, label := range []string{"配置 IPv6", "IPv6 地址", "IPv6 获取", "IPv6 网关", "IPv6 出口"} {
		if tester.HasText(label) {
			t.Fatalf("disabled IPv6 left visible: %s", label)
		}
	}
	ipRow, ipFound := tester.Find("IP 地址")
	maskRow, maskFound := tester.Find("子网掩码")
	if !ipFound || !maskFound || maskRow.Y <= ipRow.Y {
		t.Fatal("IP and subnet mask must occupy separate rows")
	}
	if tester.HasText("172.18.13.213/23 · 掩码 255.255.254.0") {
		t.Fatal("IP still includes inline prefix and subnet mask")
	}
	if err := tester.Click("复制IP 地址"); err != nil {
		t.Fatal(err)
	}
	if tester.Clipboard() != "172.18.13.213" {
		t.Fatal("IP copy contains extra fields", tester.Clipboard())
	}
	if err := tester.Click("复制子网掩码"); err != nil {
		t.Fatal(err)
	}
	if tester.Clipboard() != "255.255.254.0" {
		t.Fatal("mask copy contains extra fields", tester.Clipboard())
	}
	if err := tester.Click("选择网卡 en0"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("配置 IPv6") || !tester.HasText("fe80::1234:5678:abcd:ef01") || !tester.HasText("前缀长度") {
		t.Fatal("enabled IPv6 configuration was hidden")
	}
	tester.Scroll(1000, 650, 0, 250)
	if err := tester.Click("复制IPv6 地址"); err != nil {
		t.Fatal(err)
	}
	if tester.Clipboard() != "fe80::1234:5678:abcd:ef01" {
		t.Fatal(tester.Clipboard())
	}
	tester.Scroll(1000, 400, 0, -250)
	for _, adapter := range []string{"en8", "en0", "en8"} {
		if err := tester.Click("选择网卡 " + adapter); err != nil {
			t.Fatal(err)
		}
		if a.netconfig.selected != adapter {
			t.Fatal("adapter selection did not change", adapter)
		}
		tester.Frame()
	}
	if tester.HasText("配置 IPv6") || tester.FramePending() {
		t.Fatal("disabled IPv6 returned after switching adapters")
	}
}

func TestNetconfigMultipleAddressesKeepTheirOwnMasks(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.page = "netconfig"
	a.netconfig.snapshot = fixtureNetconfig()
	a.netconfig.snapshot.Adapters[0].Addresses = append(a.netconfig.snapshot.Adapters[0].Addresses, netconfig.Address{IP: "10.0.0.2", Prefix: 24})
	tester := ui.NewTester(a.view, 1360, 1000)
	tester.SetLocale("zh-Hans")
	if !tester.HasText("IP 地址 2") || !tester.HasText("子网掩码 2") {
		t.Fatal("secondary address fields are missing")
	}
	if err := tester.Click("复制子网掩码 2"); err != nil {
		t.Fatal(err)
	}
	if tester.Clipboard() != "255.255.255.0" {
		t.Fatal("secondary address used the first address's mask")
	}
	a.netconfig.selected = "en0"
	a.netconfig.snapshot.Adapters[1].IPv6Method = netconfig.Unknown
	tester.Frame()
	if !tester.HasText("IPv6 地址") || !tester.HasText(netconfig.Unknown) {
		t.Fatal("unknown method was treated as explicitly disabled IPv6")
	}
}

func TestNetconfigLayouts(t *testing.T) {
	for _, size := range [][2]int{{1180, 700}, {1360, 850}} {
		for _, dark := range []bool{false, true} {
			a := fixtureApp()
			a.page = "netconfig"
			a.netconfig.snapshot = fixtureNetconfig()
			tester := ui.NewTester(a.view, size[0], size[1])
			tester.SetLocale("zh-Hans")
			tester.SetDark(dark)
			tester.Frame()
			if tester.FramePending() {
				t.Fatal("network configuration view does not settle")
			}
			if directory := os.Getenv("MAC_MONITOR_SCREENSHOTS"); directory != "" {
				if err := os.MkdirAll(directory, 0755); err != nil {
					t.Fatal(err)
				}
				name := "netconfig-" + map[bool]string{false: "light", true: "dark"}[dark]
				if size[0] == 1180 {
					name += "-compact"
				}
				file, err := os.Create(filepath.Join(directory, name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, tester.Image())
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
				if size[0] == 1360 {
					tester.Scroll(1000, 650, 0, 3000)
					file, err = os.Create(filepath.Join(directory, name+"-proxy.png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tester.Image())
					file.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a.cancel()
		}
	}
}

func TestProxyOwnerRequiresLocalMatchingTCPListener(t *testing.T) {
	sockets := []connections.Connection{
		{PID: 10, Process: "Proxy", Protocol: "TCP", State: "LISTEN", Local: connections.Endpoint{IP: "127.0.0.1", Port: "7890"}},
		{PID: 11, Process: "Other", Protocol: "TCP", State: "LISTEN", Local: connections.Endpoint{IP: "192.168.1.2", Port: "7890"}},
		{PID: 12, Process: "Client", Protocol: "TCP", State: "ESTABLISHED", Local: connections.Endpoint{IP: "127.0.0.1", Port: "7890"}},
		{PID: 13, Process: "OtherV6", Protocol: "TCP", Family: "IPv6", State: "LISTEN", Local: connections.Endpoint{IP: "*", Port: "7890"}},
	}
	proxy := netconfig.Proxy{Server: "127.0.0.1", Port: 7890}
	if owner := localProxyOwner(proxy, nil, sockets); owner != "Proxy (PID 10)" {
		t.Fatal(owner)
	}
	proxy.Server = "203.0.113.8"
	if owner := localProxyOwner(proxy, nil, sockets); owner != "" {
		t.Fatal("remote proxy attributed to local process: " + owner)
	}
	proxy.Server = "192.168.1.2"
	if owner := localProxyOwner(proxy, map[netip.Addr]bool{netip.MustParseAddr("192.168.1.2"): true}, sockets); owner != "Other (PID 11)" {
		t.Fatal(owner)
	}
}
