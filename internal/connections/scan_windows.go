package connections

import (
	"context"
	"fmt"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"
	"unsafe"
)

var ipHelper = windows.NewLazySystemDLL("iphlpapi.dll")
var tcpTable = ipHelper.NewProc("GetExtendedTcpTable")
var udpTable = ipHelper.NewProc("GetExtendedUdpTable")

func readWindowsTable(ctx context.Context, protocol string, ipv6 bool) ([]Connection, error) {
	proc, class, family := tcpTable, uintptr(5), uintptr(windows.AF_INET)
	if protocol == "UDP" {
		proc, class = udpTable, 1
	}
	if ipv6 {
		family = windows.AF_INET6
	}
	if err := proc.Find(); err != nil {
		return nil, err
	}
	var size uint32
	var data []byte
	for attempt := 0; attempt < 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var ptr uintptr
		if len(data) > 0 {
			ptr = uintptr(unsafe.Pointer(&data[0]))
		}
		result, _, _ := proc.Call(ptr, uintptr(unsafe.Pointer(&size)), 0, family, class, 0)
		if result == 0 {
			return parseWindowsTable(data[:min(int(size), len(data))], protocol, ipv6)
		}
		if windows.Errno(result) != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil, windows.Errno(result)
		}
		if size < 4 || size > 16<<20 {
			return nil, fmt.Errorf("连接表超过大小限制")
		}
		data = make([]byte, size)
	}
	return nil, fmt.Errorf("连接表持续变化，请重试")
}

func Scan(parent context.Context) Snapshot {
	s := Snapshot{At: time.Now()}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	// Capture identities before the socket tables, then verify again when
	// enriching rows. A process created after the snapshot gets no identity.
	entries, err := process.ProcessesWithContext(ctx)
	if err != nil {
		s.Err = "无法读取进程身份: " + err.Error()
		return s
	}
	before := map[int32]int64{}
	for _, p := range entries {
		if ctx.Err() != nil {
			break
		}
		before[p.Pid], _ = p.CreateTimeWithContext(ctx)
	}
	var failures []string
	succeeded := 0
	for _, protocol := range []string{"TCP", "UDP"} {
		for _, ipv6 := range []bool{false, true} {
			rows, err := readWindowsTable(ctx, protocol, ipv6)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s IPv6=%t: %v", protocol, ipv6, err))
				continue
			}
			succeeded++
			s.Rows = append(s.Rows, rows...)
		}
	}
	if ctx.Err() != nil {
		s.Err = "连接采集超时或已取消"
		return s
	}
	if succeeded == 0 {
		s.Err = "连接采集失败: " + strings.Join(failures, "；")
		return s
	}
	s.Warning = strings.Join(failures, "；")
	if len(s.Rows) > MaxRows {
		s.Rows = s.Rows[:MaxRows]
		s.Warning = strings.TrimSpace(s.Warning + " 连接数量超过限制，仅显示前 16384 条")
	}
	type label struct {
		name    string
		started int64
	}
	labels := map[int32]label{}
	interfaces, _ := net.Interfaces()
	byIP := map[string]string{}
	byIndex := map[string]string{}
	for _, iface := range interfaces {
		byIndex[fmt.Sprint(iface.Index)] = iface.Name
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if prefix, err := netip.ParsePrefix(addr.String()); err == nil {
				key := prefix.Addr().Unmap().String()
				if previous, ok := byIP[key]; ok && previous != iface.Name {
					byIP[key] = ""
				} else if !ok {
					byIP[key] = iface.Name
				}
			}
		}
	}
	for i := range s.Rows {
		if ctx.Err() != nil {
			s.Err = "连接采集超时或已取消"
			return s
		}
		row := &s.Rows[i]
		l, ok := labels[row.PID]
		if !ok {
			l.name = fmt.Sprintf("PID %d", row.PID)
			if p, err := process.NewProcessWithContext(ctx, row.PID); err == nil {
				started, _ := p.CreateTimeWithContext(ctx)
				if started != 0 && started == before[row.PID] {
					l.started = started
					if name, err := p.NameWithContext(ctx); err == nil {
						l.name = name
					}
				}
			}
			labels[row.PID] = l
		}
		row.Process, row.Started = l.name, l.started
		if ip, err := netip.ParseAddr(row.Local.IP); err == nil {
			if ip.Zone() != "" {
				row.Interface = byIndex[ip.Zone()]
			} else {
				row.Interface = byIP[ip.Unmap().String()]
			}
		}
	}
	sort.Slice(s.Rows, func(i, j int) bool { return s.Rows[i].Key() < s.Rows[j].Key() })
	return s
}
