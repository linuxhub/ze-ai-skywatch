package netconfig

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func readRoutes(ctx context.Context) RouteTable {
	table := RouteTable{At: time.Now()}
	var native *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_UNSPEC, &native); err != nil {
		table.Err = "系统路由表读取失败：" + err.Error()
		return table
	}
	if native == nil {
		table.Err = "系统返回空路由表"
		return table
	}
	defer windows.FreeMibTable(unsafe.Pointer(native))
	adapters := make(map[uint32]windows.MibIfRow2)
	interfaces := make(map[[2]uint32]windows.MibIpInterfaceRow)
	for _, row := range native.Rows() {
		if err := ctx.Err(); err != nil {
			table.Err = err.Error()
			return table
		}
		adapter, found := adapters[row.InterfaceIndex]
		if !found {
			adapter = windows.MibIfRow2{InterfaceIndex: row.InterfaceIndex}
			if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormalWithoutStatistics, &adapter); err != nil {
				table.Err = "读取路由接口失败：" + err.Error()
				return table
			}
			adapters[row.InterfaceIndex] = adapter
		}
		if adapter.OperStatus != windows.IfOperStatusUp {
			continue
		}
		key := [2]uint32{row.InterfaceIndex, uint32(row.DestinationPrefix.Prefix.Family)}
		iface, found := interfaces[key]
		if !found {
			iface = windows.MibIpInterfaceRow{Family: row.DestinationPrefix.Prefix.Family, InterfaceIndex: row.InterfaceIndex}
			if err := windows.GetIpInterfaceEntry(&iface); err != nil {
				table.Err = "读取路由优先级失败：" + err.Error()
				return table
			}
			interfaces[key] = iface
		}
		if iface.Connected == 0 || row.ValidLifetime == 0 {
			continue
		}
		address, gateway := windowsRouteAddress(&row.DestinationPrefix.Prefix), windowsRouteAddress(&row.NextHop)
		prefix := netip.PrefixFrom(address, int(row.DestinationPrefix.PrefixLength))
		if !prefix.IsValid() || !gateway.IsValid() {
			table.Err = "系统返回无效路由地址"
			return table
		}
		name := windows.UTF16ToString(adapter.Alias[:])
		if name == "" {
			name = fmt.Sprintf("接口 #%d", row.InterfaceIndex)
		}
		if gateway.Is6() && gateway.IsLinkLocalUnicast() {
			gateway = gateway.WithZone(name)
		}
		description := strings.ToLower(windows.UTF16ToString(adapter.Description[:]))
		tunnel := adapter.Type == 131 || adapter.Type == 23 || adapter.TunnelType != 0
		for _, marker := range []string{"openvpn", "wireguard", "wintun", "tap-windows", "tunnel", "vpn"} {
			tunnel = tunnel || strings.Contains(description, marker)
		}
		table.Rows = append(table.Rows, RouteRule{Destination: prefix.Masked().String(), Gateway: gateway.String(), Interface: name, Metric: uint64(row.Metric) + uint64(iface.Metric), MetricKnown: true, Tunnel: tunnel})
	}
	finishRoutes(&table)
	return table
}

func windowsRouteAddress(address *windows.RawSockaddrInet) netip.Addr {
	switch address.Family {
	case windows.AF_INET:
		return netip.AddrFrom4((*windows.RawSockaddrInet4)(unsafe.Pointer(address)).Addr)
	case windows.AF_INET6:
		return netip.AddrFrom16((*windows.RawSockaddrInet6)(unsafe.Pointer(address)).Addr)
	}
	return netip.Addr{}
}
