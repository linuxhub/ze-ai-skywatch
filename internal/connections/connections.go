// Package connections reads socket snapshots independently of traffic analysis.
package connections

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linuxhub/ze-ai-skywatch/internal/network"
)

type Endpoint struct{ IP, Port string }

func (e Endpoint) String() string {
	if e.IP == "" {
		return "—"
	}
	return net.JoinHostPort(e.IP, e.Port)
}

type Connection struct {
	PID                              int32
	Started                          int64
	Process, Protocol, Family, State string
	Local, Remote                    Endpoint
	Interface                        string
	network.Traffic
}

func (c Connection) Key() string {
	return fmt.Sprintf("%d/%d/%s/%s/%s/%s", c.PID, c.Started, c.Protocol, c.Family, c.Local, c.Remote)
}
func (c Connection) Listening() bool {
	return c.State == "LISTEN" || (c.Protocol == "UDP" && c.Remote.IP == "" && c.Local.Port != "*" && c.Local.Port != "0")
}
func (c Connection) Status() string {
	if c.Protocol == "UDP" {
		if c.Remote.IP != "" {
			return "有对端"
		}
		if c.Local.Port == "*" || c.Local.Port == "0" {
			return "未绑定"
		}
		return "已绑定"
	}
	if c.State == "" {
		return "未知"
	}
	return c.State
}

// Scope classifies the peer when present, otherwise the local binding.
func (c Connection) Scope() string {
	host := c.Remote.IP
	if host == "" {
		host = c.Local.IP
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "通配 / 未知"
	}
	ip = ip.Unmap()
	if ip.IsLoopback() {
		return "本机回环"
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return "局域网"
	}
	if network.PublicIP(ip.WithZone("").String()) {
		return "公网"
	}
	return "其他地址"
}

type Snapshot struct {
	At           time.Time
	Rows         []Connection
	Err, Warning string
}

const MaxRows = 16384

type boundedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	left := (16 << 20) - b.Len()
	if n > left {
		b.overflow = true
		p = p[:left]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func parseEndpoint(s string) (Endpoint, error) {
	i := strings.LastIndexByte(s, ':')
	if i < 1 {
		return Endpoint{}, fmt.Errorf("无法解析连接端点：%q", s)
	}
	host, port := strings.Trim(s[:i], "[]"), s[i+1:]
	if host != "*" {
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return Endpoint{}, fmt.Errorf("无法解析连接 IP：%q", host)
		}
		host = ip.String()
	}
	if port != "*" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 {
			return Endpoint{}, fmt.Errorf("无法解析连接端口：%q", port)
		}
		port = strconv.Itoa(n)
	}
	return Endpoint{host, port}, nil
}

// Parse preserves address families, TCP states, unconnected UDP and scoped IPv6.
// Multiple file descriptors for the same endpoint are collapsed.
func Parse(data []byte) ([]Connection, error) {
	rows := []Connection{}
	seen := map[string]bool{}
	var pid int32
	var name string
	var current Connection
	haveFile := false
	flush := func() {
		if !haveFile || current.Local.IP == "" || (current.Protocol != "TCP" && current.Protocol != "UDP") {
			return
		}
		key := current.Key()
		if !seen[key] {
			seen[key] = true
			rows = append(rows, current)
		}
	}
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		line := scan.Text()
		if line == "" {
			continue
		}
		value := line[1:]
		switch line[0] {
		case 'p':
			flush()
			haveFile = false
			name = ""
			n, err := strconv.ParseInt(value, 10, 32)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("无法解析连接 PID：%q", value)
			}
			pid = int32(n)
		case 'c':
			name = value
		case 'f':
			flush()
			current = Connection{PID: pid, Process: name}
			haveFile = true
		case 't':
			current.Family = value
		case 'P':
			current.Protocol = value
		case 'T':
			if strings.HasPrefix(value, "ST=") {
				current.State = strings.TrimPrefix(value, "ST=")
			}
		case 'n':
			if !haveFile {
				continue
			}
			parts := strings.Split(value, "->")
			if len(parts) > 2 {
				return nil, fmt.Errorf("无法解析连接地址：%q", value)
			}
			local, err := parseEndpoint(parts[0])
			if err != nil {
				return nil, err
			}
			current.Local = local
			if len(parts) == 2 {
				remote, err := parseEndpoint(parts[1])
				if err != nil {
					return nil, err
				}
				current.Remote = remote
			}
		}
	}
	flush()
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("读取连接数据失败：%w", err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key() < rows[j].Key() })
	return rows, nil
}
