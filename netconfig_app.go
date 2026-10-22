package main

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
	"github.com/linuxhub/ze-ai-skywatch/internal/netconfig"
)

type netconfigState struct {
	snapshot                    netconfig.Snapshot
	selected                    string
	showAll                     bool
	running, scanning, checking bool
	cancel                      context.CancelFunc
	generation, checkGeneration uint64
	refresh                     chan struct{}
	checks                      map[string]string
	checkCancel                 context.CancelFunc
	routeFilter, routeQuery     string
	routeList                   ui.ListState
	routeMatcher                *netconfig.RouteMatcher
	routeMatches                map[string]netconfig.RouteMatch
	tab                         int
}

func (a *app) syncNetconfig() {
	state := &a.netconfig
	visible := (a.page == "netconfig" || a.page == "connections") && a.network.visible && a.win != nil
	if !visible && state.running {
		state.generation++
		state.cancel()
		state.cancel = nil
		state.running, state.scanning = false, false
		a.cancelProxyCheck()
	}
	if !visible || state.running {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	state.cancel, state.running, state.scanning = cancel, true, true
	state.generation++
	state.refresh = make(chan struct{}, 1)
	generation, refresh, win := state.generation, state.refresh, a.win
	go func() {
		defer cancel()
		collect := func() {
			win.Update(func() {
				if a.netconfig.generation == generation {
					a.netconfig.scanning = true
				}
			})
			limited, stop := context.WithTimeout(ctx, 15*time.Second)
			snapshot := netconfig.Scan(limited)
			stop()
			if ctx.Err() != nil {
				return
			}
			win.Update(func() { a.publishNetconfig(generation, snapshot) })
		}
		collect()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collect()
			case <-refresh:
				collect()
			}
		}
	}()
}

func (a *app) publishNetconfig(generation uint64, snapshot netconfig.Snapshot) {
	state := &a.netconfig
	if generation != state.generation {
		return
	}
	state.scanning = false
	if snapshot.Err != "" {
		state.snapshot.Err = snapshot.Err
		state.snapshot.Routing.Err = "网络配置采集失败，路由状态未更新"
		state.routeMatcher, state.routeMatches = nil, nil
		a.connections.rows = nil
		return
	}
	a.cancelProxyCheck()
	state.checks = nil
	if snapshot.Routing.Err != "" && !state.snapshot.Routing.At.IsZero() {
		previous := state.snapshot.Routing
		previous.Err = snapshot.Routing.Err
		snapshot.Routing = previous
	}
	state.snapshot = snapshot
	state.routeMatcher, state.routeMatches = nil, nil
	a.connections.rows = nil
	rows := a.netconfigAdapters()
	for _, row := range rows {
		if row.Name == state.selected {
			return
		}
	}
	state.selected = ""
	if len(rows) > 0 {
		state.selected = rows[0].Name
	}
}

func (a *app) netconfigAdapters() []netconfig.Adapter {
	var rows []netconfig.Adapter
	routed := make(map[string]bool)
	for _, rule := range a.netconfig.snapshot.Routing.Rows {
		if rule.TunnelPolicy() {
			routed[rule.Interface] = true
		}
	}
	for _, adapter := range a.netconfig.snapshot.Adapters {
		isDefault := adapter.Name == a.netconfig.snapshot.Default4.Interface || adapter.Name == a.netconfig.snapshot.Default6.Interface
		if a.netconfig.showAll || isDefault || routed[adapter.Name] || (adapter.Up && !adapter.Virtual) {
			rows = append(rows, adapter)
		}
	}
	return rows
}

func (a *app) matchRoute(destination string) netconfig.RouteMatch {
	state := &a.netconfig
	if state.routeMatcher == nil {
		state.routeMatcher = netconfig.NewRouteMatcher(state.snapshot.Routing)
		state.routeMatches = make(map[string]netconfig.RouteMatch)
	}
	if match, found := state.routeMatches[destination]; found {
		return match
	}
	match := state.routeMatcher.Match(destination)
	if len(state.routeMatches) < netconfig.MaxRoutes {
		state.routeMatches[destination] = match
	}
	return match
}

func (a *app) filteredRoutes() []netconfig.RouteRule {
	var rows []netconfig.RouteRule
	for _, rule := range a.netconfig.snapshot.Routing.Rows {
		if a.netconfig.routeFilter == "VPN／隧道路由" && !rule.TunnelPolicy() {
			continue
		}
		if a.netconfig.routeFilter == "IPv4 路由" && strings.Contains(rule.Destination, ":") {
			continue
		}
		if a.netconfig.routeFilter == "IPv6 路由" && !strings.Contains(rule.Destination, ":") {
			continue
		}
		rows = append(rows, rule)
	}
	return rows
}

func (a *app) cancelProxyCheck() {
	state := &a.netconfig
	state.checkGeneration++
	if state.checkCancel != nil {
		state.checkCancel()
		state.checkCancel = nil
	}
	state.checking = false
}

func (a *app) checkNetconfigProxies() {
	if a.win == nil || a.netconfig.checking {
		return
	}
	a.cancelProxyCheck()
	state := &a.netconfig
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	state.checkCancel, state.checking = cancel, true
	proxies := append([]netconfig.Proxy(nil), state.snapshot.Proxies...)
	addresses := make(map[netip.Addr]bool)
	for _, adapter := range state.snapshot.Adapters {
		for _, address := range adapter.Addresses {
			if ip, err := netip.ParseAddr(address.IP); err == nil {
				addresses[ip.Unmap()] = true
			}
		}
	}
	generation, win := state.checkGeneration, a.win
	go func() {
		defer cancel()
		checks := make(map[string]string)
		local := false
		for _, proxy := range proxies {
			if proxy.Known && proxy.Enabled && proxy.Server != "" && proxy.Port > 0 {
				checks[proxy.Kind] = netconfig.CheckProxy(ctx, proxy)
				ip, err := netip.ParseAddr(proxy.Server)
				local = local || strings.EqualFold(proxy.Server, "localhost") || (err == nil && (ip.IsLoopback() || addresses[ip.Unmap()]))
			}
		}
		if local && ctx.Err() == nil {
			sockets := connections.Scan(ctx)
			for _, proxy := range proxies {
				if checks[proxy.Kind] == "" {
					continue
				}
				if owner := localProxyOwner(proxy, addresses, sockets.Rows); owner != "" {
					checks[proxy.Kind] += " · " + owner
				}
			}
		}
		win.Update(func() {
			if a.netconfig.checkGeneration != generation {
				return
			}
			a.netconfig.checks, a.netconfig.checking, a.netconfig.checkCancel = checks, false, nil
		})
	}()
}

func localProxyOwner(proxy netconfig.Proxy, addresses map[netip.Addr]bool, sockets []connections.Connection) string {
	ip, err := netip.ParseAddr(proxy.Server)
	ip = ip.Unmap()
	localhost := strings.EqualFold(proxy.Server, "localhost")
	if !localhost && (err != nil || (!ip.IsLoopback() && !addresses[ip.Unmap()])) {
		return ""
	}
	var owners []string
	for _, socket := range sockets {
		if socket.Protocol != "TCP" || socket.State != "LISTEN" || socket.Local.Port != strconv.Itoa(proxy.Port) {
			continue
		}
		bound, boundErr := netip.ParseAddr(socket.Local.IP)
		wildcard := socket.Local.IP == "*" && (localhost || (ip.Is4() && socket.Family == "IPv4") || (ip.Is6() && socket.Family == "IPv6"))
		matches := wildcard || (boundErr == nil && ((localhost && (bound.IsLoopback() || bound.IsUnspecified())) || (!localhost && (bound.Unmap() == ip || (bound.IsUnspecified() && bound.Is4() == ip.Is4())))))
		if !matches {
			continue
		}
		owner := fmt.Sprintf("%s (PID %d)", socket.Process, socket.PID)
		duplicate := false
		for _, existing := range owners {
			duplicate = duplicate || existing == owner
		}
		if !duplicate {
			owners = append(owners, owner)
		}
	}
	return strings.Join(owners, "、")
}
