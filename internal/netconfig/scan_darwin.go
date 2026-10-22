package netconfig

import (
	"context"
	"net/netip"
	"strconv"
	"strings"
)

type macService struct {
	name, device, kind string
	disabled           bool
}

func Scan(ctx context.Context) Snapshot {
	snapshot := baseSnapshot()
	if err := ctx.Err(); err != nil {
		snapshot.Err = "网络配置采集未完成：" + err.Error()
	}
	if snapshot.Err != "" {
		return snapshot
	}
	services, err := command(ctx, "/usr/sbin/networksetup", "-listnetworkserviceorder")
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, "网络服务配置不可读，IP／DNS 获取方式可能无法确定")
	}
	serviceMap := parseServices(services)
	for index := range snapshot.Adapters {
		adapter := &snapshot.Adapters[index]
		service, exists := serviceMap[adapter.Name]
		if !exists {
			continue
		}
		adapter.Service, adapter.Kind = service.name, service.kind
		adapter.Virtual = !strings.HasPrefix(adapter.Name, "en")
		if service.disabled {
			adapter.Status, adapter.Up = "已禁用", false
		}
		info, infoErr := command(ctx, "/usr/sbin/networksetup", "-getinfo", service.name)
		if infoErr == nil {
			applyMacInfo(adapter, info)
		} else {
			snapshot.Warnings = append(snapshot.Warnings, service.name+" IP 配置不可读")
		}
		dns, dnsErr := command(ctx, "/usr/sbin/networksetup", "-getdnsservers", service.name)
		if dnsErr == nil {
			adapter.DNS, adapter.DNSMode = parseManualDNS(dns)
		} else {
			snapshot.Warnings = append(snapshot.Warnings, service.name+" DNS 配置不可读")
		}
	}
	for _, family := range []string{"-inet", "-inet6"} {
		output, routeErr := command(ctx, "/sbin/route", "-n", "get", family, "default")
		if routeErr != nil {
			if !strings.Contains(output, "not in table") {
				snapshot.Warnings = append(snapshot.Warnings, "默认路由未获取（"+family+"，可能未配置）")
			}
			continue
		}
		route := parseRoute(output)
		if family == "-inet" {
			snapshot.Default4 = route
		} else {
			snapshot.Default6 = route
		}
		for index := range snapshot.Adapters {
			adapter := &snapshot.Adapters[index]
			if adapter.Name != route.Interface {
				continue
			}
			if family == "-inet" {
				adapter.Gateway4 = appendUnique(adapter.Gateway4, route.Gateway)
			} else {
				adapter.Gateway6 = appendUnique(adapter.Gateway6, route.Gateway)
			}
		}
	}
	dns, dnsErr := command(ctx, "/usr/sbin/scutil", "--dns")
	if dnsErr == nil && strings.Contains(dns, "resolver #") {
		var scoped map[string][]string
		snapshot.DNS, scoped = parseResolvers(dns)
		for index := range snapshot.Adapters {
			adapter := &snapshot.Adapters[index]
			if servers := scoped[adapter.Name]; len(servers) > 0 {
				if adapter.DNSMode == "手动配置" && strings.Join(adapter.DNS, ",") != strings.Join(servers, ",") {
					adapter.DNSMode = "手动配置；有效 DNS 已由系统覆盖"
				}
				adapter.DNS = servers
			}
		}
	} else {
		snapshot.Warnings = append(snapshot.Warnings, "系统有效 DNS 不可读或未配置")
	}
	proxy, proxyErr := command(ctx, "/usr/sbin/scutil", "--proxy")
	snapshot.ProxyScope = "macOS 当前系统代理；应用自设代理与隧道转发单独判断"
	snapshot.Proxies, snapshot.Exceptions = parseMacProxies(proxy)
	snapshot.Routing = readRoutes(ctx)
	if proxyErr != nil || snapshot.ProxyStatus() == Unknown {
		snapshot.Warnings = append(snapshot.Warnings, "系统代理配置未能完整读取")
	}
	if err := ctx.Err(); err != nil {
		snapshot.Err = "网络配置采集未完成：" + err.Error()
	}
	sortAdapters(&snapshot)
	return snapshot
}

func parseServices(output string) map[string]macService {
	services := make(map[string]macService)
	var current macService
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "(") && !strings.HasPrefix(line, "(Hardware Port:") {
			if end := strings.Index(line, ") "); end >= 0 {
				name := line[end+2:]
				current = macService{name: strings.TrimPrefix(name, "*"), disabled: strings.HasPrefix(name, "*") || strings.HasPrefix(line, "(*)")}
			}
		}
		if strings.HasPrefix(line, "(Hardware Port:") {
			fields := strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(line, "(Hardware Port: "), ")"), ", Device: ", 2)
			if len(fields) == 2 && fields[1] != "" {
				current.kind, current.device = fields[0], fields[1]
				services[current.device] = current
			}
		}
	}
	return services
}

func applyMacInfo(adapter *Adapter, output string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "DHCP Configuration":
			adapter.IPv4Method = "DHCP 自动获取"
		case "Manual Configuration":
			adapter.IPv4Method = "手动固定 IP"
		case "BOOTP Configuration":
			adapter.IPv4Method = "BOOTP"
		case "DHCP with manual address":
			adapter.IPv4Method = "DHCP + 手动地址"
		}
		key, value, found := strings.Cut(line, ": ")
		if !found {
			continue
		}
		if key == "Router" {
			if address, err := netip.ParseAddr(value); err == nil && !address.IsUnspecified() {
				adapter.Gateway4 = appendUnique(adapter.Gateway4, value)
			}
		}
		if key == "IPv6" {
			switch value {
			case "Off":
				adapter.IPv6Method = "已关闭"
			case "Automatic":
				adapter.IPv6Method = "自动配置（SLAAC／DHCPv6）"
			case "Manual":
				adapter.IPv6Method = "手动配置"
			case "Link-local only":
				adapter.IPv6Method = "仅链路本地"
			}
		}
		if key == "IPv6 Router" {
			if value != "none" {
				adapter.Gateway6 = appendUnique(adapter.Gateway6, value)
			}
		}
	}
}

func parseManualDNS(output string) ([]string, string) {
	var servers []string
	for _, line := range strings.Fields(output) {
		if address, err := netip.ParseAddr(line); err == nil {
			servers = appendUnique(servers, address.String())
		}
	}
	if len(servers) > 0 {
		return servers, "手动配置"
	}
	if strings.Contains(output, "There aren't any DNS Servers set") {
		return nil, "自动／未手动指定"
	}
	return nil, Unknown
}

func parseRoute(output string) Route {
	var route Route
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found {
			continue
		}
		if key == "gateway" {
			route.Gateway = strings.TrimSpace(value)
		}
		if key == "interface" {
			route.Interface = strings.TrimSpace(value)
		}
	}
	return route
}

func parseResolvers(output string) ([]string, map[string][]string) {
	var global, servers []string
	scoped := make(map[string][]string)
	iface, domain := "", ""
	resolverScoped := false
	scopedSection := false
	flush := func() {
		if iface != "" && domain == "" {
			for _, server := range servers {
				scoped[iface] = appendUnique(scoped[iface], server)
			}
		}
		if domain == "" && !scopedSection && !resolverScoped {
			for _, server := range servers {
				global = appendUnique(global, server)
			}
		}
		servers, iface, domain = nil, "", ""
		resolverScoped = false
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "DNS configuration (") {
			flush()
			scopedSection = true
		}
		if strings.HasPrefix(line, "resolver #") {
			flush()
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if strings.HasPrefix(key, "nameserver[") {
			if _, err := netip.ParseAddr(value); err == nil {
				servers = appendUnique(servers, value)
			}
		}
		if key == "domain" {
			domain = value
		}
		if key == "flags" {
			resolverScoped = strings.Contains(value, "Scoped") || strings.Contains(value, "Supplemental")
		}
		if key == "if_index" {
			if start := strings.Index(value, "("); start >= 0 {
				iface = strings.TrimSuffix(value[start+1:], ")")
			}
		}
	}
	flush()
	return global, scoped
}

func parseMacProxies(output string) ([]Proxy, []string) {
	values := make(map[string]string)
	var exceptions []string
	depth, exceptionDepth := 0, -1
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "{") {
			depth++
			if depth == 2 && strings.HasPrefix(line, "ExceptionsList") {
				exceptionDepth = depth
			}
			continue
		}
		if line == "}" {
			if depth == exceptionDepth {
				exceptionDepth = -1
			}
			depth--
			continue
		}
		key, value, found := strings.Cut(line, " : ")
		if !found {
			continue
		}
		if exceptionDepth == depth {
			exceptions = append(exceptions, value)
		} else if depth == 1 {
			values[key] = value
		}
	}
	var proxies []Proxy
	for _, kind := range []string{"HTTP", "HTTPS", "SOCKS", "PAC", "自动发现"} {
		flag := kind + "Enable"
		if kind == "PAC" {
			flag = "ProxyAutoConfigEnable"
		}
		if kind == "自动发现" {
			flag = "ProxyAutoDiscoveryEnable"
		}
		_, known := values[flag]
		if len(values) > 0 {
			known = true
		}
		proxy := Proxy{Kind: kind, Known: known, Enabled: values[flag] == "1", Server: values[kind+"Proxy"]}
		proxy.Port, _ = strconv.Atoi(values[kind+"Port"])
		if kind == "PAC" {
			proxy.URL = safeURL(values["ProxyAutoConfigURLString"])
		}
		proxies = append(proxies, proxy)
	}
	return proxies, exceptions
}
