package netconfig

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const Unknown = "无法确定"

type Address struct {
	IP     string
	Prefix int
	Method string
}

func (address Address) SubnetMask() string {
	ip, err := netip.ParseAddr(address.IP)
	if err != nil || !ip.Is4() || address.Prefix < 0 || address.Prefix > 32 {
		return ""
	}
	return net.IP(net.CIDRMask(address.Prefix, 32)).String()
}

func (address Address) String() string {
	ip, err := netip.ParseAddr(address.IP)
	if err != nil || address.Prefix < 0 || address.Prefix > ip.BitLen() {
		return address.IP
	}
	value := fmt.Sprintf("%s/%d", address.IP, address.Prefix)
	if ip.Is4() {
		value += " · 掩码 " + address.SubnetMask()
	}
	if address.Method != "" {
		value += " · " + address.Method
	}
	return value
}

type Adapter struct {
	Index                            int
	Name, Service, Kind, Status, MAC string
	MTU                              int
	Virtual, Up                      bool
	Addresses                        []Address
	IPv4Method, IPv6Method           string
	Gateway4, Gateway6               []string
	DNS                              []string
	DNSMode                          string
}

type Route struct {
	Interface, Gateway string
}

type Proxy struct {
	Kind           string
	Known, Enabled bool
	Server         string
	Port           int
	URL            string
}

func (proxy Proxy) Status() string {
	if !proxy.Known {
		return Unknown
	}
	if proxy.Enabled {
		return "已启用"
	}
	return "未启用"
}

func (proxy Proxy) Endpoint() string {
	if proxy.URL != "" {
		return safeURL(proxy.URL)
	}
	if proxy.Server != "" && proxy.Port > 0 {
		return net.JoinHostPort(proxy.Server, strconv.Itoa(proxy.Port))
	}
	return proxy.Server
}

func safeURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "地址无法解析"
	}
	parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
	return parsed.String()
}

type Snapshot struct {
	At                 time.Time
	Adapters           []Adapter
	Default4, Default6 Route
	DNS                []string
	Proxies            []Proxy
	Exceptions         []string
	ProxyScope         string
	Warnings           []string
	Err                string
	Routing            RouteTable
}

func (snapshot Snapshot) ProxyStatus() string {
	known := false
	unknown := false
	for _, proxy := range snapshot.Proxies {
		if proxy.Known && proxy.Enabled {
			return "已启用"
		}
		known = known || proxy.Known
		unknown = unknown || !proxy.Known
	}
	if unknown {
		return Unknown
	}
	if known {
		return "未启用"
	}
	return Unknown
}

func (snapshot Snapshot) Text() string {
	var output strings.Builder
	fmt.Fprintf(&output, "网络配置 · %s\nIPv4 默认出口：%s · %s\nIPv6 默认出口：%s · %s\n系统 DNS：%s\n", snapshot.At.Format("2006-01-02 15:04:05"), value(snapshot.Default4.Interface), value(snapshot.Default4.Gateway), value(snapshot.Default6.Interface), value(snapshot.Default6.Gateway), value(strings.Join(snapshot.DNS, "、")))
	for _, adapter := range snapshot.Adapters {
		fmt.Fprintf(&output, "\n%s (%s) · %s · %s\nMAC：%s · MTU：%d\nIPv4 配置：%s\nIPv6 配置：%s\n", adapter.Service, adapter.Name, adapter.Kind, adapter.Status, value(adapter.MAC), adapter.MTU, value(adapter.IPv4Method), value(adapter.IPv6Method))
		for _, address := range adapter.Addresses {
			fmt.Fprintf(&output, "IP：%s\n", address.String())
		}
		fmt.Fprintf(&output, "IPv4 网关：%s\nIPv6 网关：%s\nDNS：%s\nDNS 获取方式：%s\n", value(strings.Join(adapter.Gateway4, "、")), value(strings.Join(adapter.Gateway6, "、")), value(strings.Join(adapter.DNS, "、")), value(adapter.DNSMode))
	}
	fmt.Fprintf(&output, "\n代理范围：%s\n", snapshot.ProxyScope)
	for _, proxy := range snapshot.Proxies {
		fmt.Fprintf(&output, "%s：%s %s\n", proxy.Kind, proxy.Status(), proxy.Endpoint())
	}
	fmt.Fprintf(&output, "绕过列表：%s\n实际流量是否经过代理：无法确定\n", value(strings.Join(snapshot.Exceptions, "、")))
	for _, warning := range snapshot.Warnings {
		fmt.Fprintf(&output, "提示：%s\n", warning)
	}
	output.WriteString("\n" + snapshot.Routing.Text())
	if snapshot.Err != "" {
		fmt.Fprintf(&output, "采集失败：%s\n", snapshot.Err)
	}
	return output.String()
}

func value(text string) string {
	if text == "" {
		return "—"
	}
	return text
}

func baseSnapshot() Snapshot {
	snapshot := Snapshot{At: time.Now()}
	interfaces, err := net.Interfaces()
	if err != nil {
		snapshot.Err = "读取网卡失败：" + err.Error()
		return snapshot
	}
	for _, iface := range interfaces {
		adapter := Adapter{Index: iface.Index, Name: iface.Name, Service: iface.Name, Kind: "虚拟／其他", MAC: iface.HardwareAddr.String(), MTU: iface.MTU, Virtual: true, IPv4Method: Unknown, IPv6Method: Unknown, DNSMode: Unknown, Status: "未连接"}
		adapter.Up = iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagRunning != 0
		if iface.Flags&net.FlagUp == 0 {
			adapter.Status = "未启用"
		} else if adapter.Up {
			adapter.Status = "已连接"
		}
		if iface.Flags&net.FlagLoopback != 0 {
			adapter.Kind = "回环"
		}
		if strings.HasPrefix(iface.Name, "utun") || strings.HasPrefix(iface.Name, "tun") || strings.HasPrefix(iface.Name, "ppp") {
			adapter.Kind = "隧道／VPN 接口"
		}
		addresses, addressErr := iface.Addrs()
		if addressErr != nil {
			snapshot.Warnings = append(snapshot.Warnings, iface.Name+" 地址读取失败")
		}
		for _, address := range addresses {
			if prefix, err := netip.ParsePrefix(address.String()); err == nil {
				adapter.Addresses = append(adapter.Addresses, Address{IP: prefix.Addr().String(), Prefix: prefix.Bits()})
			}
		}
		snapshot.Adapters = append(snapshot.Adapters, adapter)
	}
	return snapshot
}

func sortAdapters(snapshot *Snapshot) {
	sort.SliceStable(snapshot.Adapters, func(first, second int) bool {
		left, right := snapshot.Adapters[first], snapshot.Adapters[second]
		leftDefault := left.Name == snapshot.Default4.Interface || left.Name == snapshot.Default6.Interface
		rightDefault := right.Name == snapshot.Default4.Interface || right.Name == snapshot.Default6.Interface
		if leftDefault != rightDefault {
			return leftDefault
		}
		if left.Up != right.Up {
			return left.Up
		}
		if left.Virtual != right.Virtual {
			return !left.Virtual
		}
		return left.Index < right.Index
	})
}

func command(ctx context.Context, executable string, args ...string) (string, error) {
	limited, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(limited, executable, args...).Output()
	return strings.TrimSpace(string(output)), err
}

func appendUnique(values []string, candidate string) []string {
	if candidate == "" {
		return values
	}
	for _, existing := range values {
		if existing == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func CheckProxy(ctx context.Context, proxy Proxy) string {
	if !proxy.Known || !proxy.Enabled || proxy.Server == "" || proxy.Port < 1 || proxy.Port > 65535 {
		return "无可检测的固定代理端点"
	}
	connection, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(proxy.Server, strconv.Itoa(proxy.Port)))
	if err != nil {
		return "TCP 无法连接（服务未运行、网络或防火墙限制）"
	}
	connection.Close()
	return "TCP 可连接（未验证代理协议或实际流量）"
}
