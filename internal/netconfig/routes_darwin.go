package netconfig

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

func readRoutes(ctx context.Context) RouteTable {
	table := RouteTable{At: time.Now()}
	for _, family := range []string{"inet", "inet6"} {
		output, err := command(ctx, "/usr/sbin/netstat", "-rn", "-f", family)
		if err != nil {
			table.Err = "系统路由表读取失败（" + family + "）：" + err.Error()
			return table
		}
		rows, err := parseRouteTable(output, family == "inet6")
		if err != nil {
			table.Err = err.Error()
			return table
		}
		table.Rows = append(table.Rows, rows...)
	}
	finishRoutes(&table)
	return table
}

func parseRouteTable(output string, ipv6 bool) ([]RouteRule, error) {
	var rows []RouteRule
	header := false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "Destination" {
			header = true
			continue
		}
		if !header {
			continue
		}
		if len(fields) < 4 {
			return nil, fmt.Errorf("无法识别路由记录：%s", line)
		}
		flags, iface := fields[2], fields[3]
		if !strings.Contains(flags, "U") {
			continue
		}
		if (strings.ContainsAny(flags, "LW")) && !strings.HasPrefix(iface, "lo") {
			continue
		}
		prefix, err := macRoutePrefix(fields[0], ipv6)
		if err != nil {
			return nil, fmt.Errorf("无法识别路由网段 %s", fields[0])
		}
		rows = append(rows, RouteRule{Destination: prefix.String(), Gateway: fields[1], Flags: flags, Interface: iface, Scoped: strings.Contains(flags, "I"), Reject: strings.ContainsAny(flags, "RB"), Tunnel: tunnelInterface(iface)})
	}
	if !header {
		return nil, fmt.Errorf("系统路由表缺少表头")
	}
	return rows, nil
}

func macRoutePrefix(destination string, ipv6 bool) (netip.Prefix, error) {
	if destination == "default" {
		if ipv6 {
			return netip.MustParsePrefix("::/0"), nil
		}
		return netip.MustParsePrefix("0.0.0.0/0"), nil
	}
	host, mask, explicit := strings.Cut(destination, "/")
	host, _, _ = strings.Cut(host, "%")
	bits := 128
	if !ipv6 {
		parts := strings.Split(host, ".")
		if len(parts) > 4 {
			return netip.Prefix{}, fmt.Errorf("invalid IPv4")
		}
		bits = len(parts) * 8
		for len(parts) < 4 {
			parts = append(parts, "0")
		}
		host = strings.Join(parts, ".")
	}
	if explicit {
		parsed, err := strconv.Atoi(mask)
		if err != nil {
			return netip.Prefix{}, err
		}
		bits = parsed
	}
	address, err := netip.ParseAddr(host)
	if err != nil || address.Is6() != ipv6 {
		return netip.Prefix{}, fmt.Errorf("invalid route address")
	}
	prefix := netip.PrefixFrom(address, bits)
	if !prefix.IsValid() {
		return netip.Prefix{}, fmt.Errorf("invalid prefix length")
	}
	return prefix.Masked(), nil
}
