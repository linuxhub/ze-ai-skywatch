package netconfig

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"
)

const MaxRoutes = 8192

type RouteRule struct {
	Destination, Gateway, Interface, Flags string
	Metric                                 uint64
	Scoped, Reject, Tunnel                 bool
	MetricKnown                            bool
}

func (rule RouteRule) Key() string {
	return fmt.Sprintf("%s/%s/%s/%s/%d/%t/%t", rule.Destination, rule.Gateway, rule.Interface, rule.Flags, rule.Metric, rule.Scoped, rule.Reject)
}

func (rule RouteRule) Path() string {
	if rule.Reject {
		return "拒绝／丢弃"
	}
	if rule.Tunnel {
		return "VPN／隧道"
	}
	if strings.HasPrefix(rule.Interface, "lo") || strings.Contains(strings.ToLower(rule.Interface), "loopback") {
		return "本机回环"
	}
	return "普通接口"
}

func (rule RouteRule) NextHop() string {
	if strings.HasPrefix(rule.Gateway, "link#") || rule.Gateway == "0.0.0.0" || rule.Gateway == "::" {
		return "直连"
	}
	return value(rule.Gateway)
}

func (rule RouteRule) TunnelPolicy() bool {
	prefix, err := netip.ParsePrefix(rule.Destination)
	return err == nil && rule.Tunnel && !rule.Scoped && !rule.Reject && !prefix.Addr().IsMulticast() && !prefix.Addr().IsLinkLocalUnicast()
}

func (rule RouteRule) MetricText() string {
	if !rule.MetricKnown {
		return "—"
	}
	return fmt.Sprint(rule.Metric)
}

type RouteTable struct {
	At        time.Time
	Rows      []RouteRule
	Err       string
	Truncated bool
}

func (table RouteTable) Text() string {
	var output strings.Builder
	fmt.Fprintf(&output, "路由与 VPN 分流 · %s\n目标网段\t网关\t出口\t走向\t作用域\t总跃点\n", table.At.Format("15:04:05"))
	for _, rule := range table.Rows {
		scope := "通用"
		if rule.Scoped {
			scope = "仅限接口"
		}
		fmt.Fprintf(&output, "%s\t%s\t%s\t%s\t%s\t%s\n", rule.Destination, rule.NextHop(), rule.Interface, rule.Path(), scope, rule.MetricText())
	}
	if table.Err != "" {
		fmt.Fprintf(&output, "采集失败：%s\n", table.Err)
	}
	if table.Truncated {
		output.WriteString("路由数量超过上限，目标出口推断暂停。\n")
	}
	output.WriteString("按当前路由规则推断系统出口；应用绑定接口、既有连接及应用自设代理可能不同，不代表实际报文路径。\n")
	return output.String()
}

func finishRoutes(table *RouteTable) {
	seen := make(map[string]bool)
	rows := make([]RouteRule, 0, len(table.Rows))
	for _, rule := range table.Rows {
		prefix, err := netip.ParsePrefix(rule.Destination)
		if err != nil || rule.Interface == "" {
			table.Err = "路由数据包含无法识别的网段或接口"
			return
		}
		rule.Destination = prefix.Masked().String()
		if seen[rule.Key()] {
			continue
		}
		seen[rule.Key()] = true
		rows = append(rows, rule)
	}
	sort.SliceStable(rows, func(first, second int) bool {
		left, right := rows[first], rows[second]
		if left.TunnelPolicy() != right.TunnelPolicy() {
			return left.TunnelPolicy()
		}
		if left.Destination != right.Destination {
			return left.Destination < right.Destination
		}
		if left.Metric != right.Metric {
			return left.Metric < right.Metric
		}
		return left.Key() < right.Key()
	})
	if len(rows) > MaxRoutes {
		rows, table.Truncated = rows[:MaxRoutes], true
	}
	table.Rows = rows
}

type RouteMatch struct {
	Rule   RouteRule
	Found  bool
	Reason string
}

func (match RouteMatch) Summary() string {
	if !match.Found {
		return match.Reason
	}
	return match.Rule.Path() + " · " + match.Rule.Interface
}

func (match RouteMatch) Text() string {
	if !match.Found {
		return match.Reason
	}
	return fmt.Sprintf("%s → %s → %s（%s）", match.Rule.Destination, match.Rule.NextHop(), match.Rule.Interface, match.Rule.Path())
}

type RouteMatcher struct {
	rules  map[netip.Prefix][]RouteRule
	reason string
}

func NewRouteMatcher(table RouteTable) *RouteMatcher {
	matcher := &RouteMatcher{rules: make(map[netip.Prefix][]RouteRule)}
	if table.Err != "" {
		matcher.reason = "路由采集失败，暂不推断"
		return matcher
	}
	if table.Truncated {
		matcher.reason = "路由不完整，暂不推断"
		return matcher
	}
	if table.At.IsZero() {
		matcher.reason = "等待路由采集"
		return matcher
	}
	for _, rule := range table.Rows {
		prefix, err := netip.ParsePrefix(rule.Destination)
		if err != nil {
			matcher.reason = "路由数据不可用"
			return matcher
		}
		prefix = prefix.Masked()
		matcher.rules[prefix] = append(matcher.rules[prefix], rule)
	}
	return matcher
}

func (matcher *RouteMatcher) Match(destination string) RouteMatch {
	if destination == "" || destination == "*" {
		return RouteMatch{Reason: "无远端地址"}
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(destination))
	if err != nil {
		return RouteMatch{Reason: "请输入有效的 IPv4／IPv6 地址"}
	}
	if ip.IsUnspecified() {
		return RouteMatch{Reason: "目标为未指定地址"}
	}
	if matcher == nil {
		return RouteMatch{Reason: "等待路由采集"}
	}
	if matcher.reason != "" {
		return RouteMatch{Reason: matcher.reason}
	}
	zone := ip.Zone()
	ip = ip.WithZone("").Unmap()
	if ip.IsLinkLocalUnicast() && ip.Is6() && zone == "" {
		return RouteMatch{Reason: "链路本地 IPv6 需要指定 %接口"}
	}
	for bits := ip.BitLen(); bits >= 0; bits-- {
		prefix := netip.PrefixFrom(ip, bits).Masked()
		var best RouteRule
		found, ambiguous := false, false
		for _, rule := range matcher.rules[prefix] {
			if zone == "" && rule.Scoped {
				continue
			}
			if zone != "" && rule.Interface != zone {
				continue
			}
			if !found || rule.Metric < best.Metric {
				best, found, ambiguous = rule, true, false
				continue
			}
			if rule.Metric == best.Metric && (rule.Interface != best.Interface || rule.Gateway != best.Gateway || rule.Reject != best.Reject) {
				ambiguous = true
			}
		}
		if ambiguous {
			return RouteMatch{Reason: "同优先级存在多个出口，无法唯一确定"}
		}
		if found {
			return RouteMatch{Rule: best, Found: true}
		}
	}
	return RouteMatch{Reason: "没有匹配的通用路由"}
}

func tunnelInterface(name string) bool {
	for _, prefix := range []string{"utun", "tun", "tap", "ppp", "ipsec"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
