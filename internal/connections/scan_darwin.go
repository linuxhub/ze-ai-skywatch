package connections

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"net/netip"
	"os/exec"
	"strings"
	"time"
)

// Scan never invokes a shell, DNS lookup, or privilege escalation.
func Scan(parent context.Context) Snapshot {
	s := Snapshot{At: time.Now()}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-iTCP", "-iUDP", "-FpcftPnT")
	var out, stderr boundedBuffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		s.Err = "连接采集超时或已取消"
		return s
	}
	if out.overflow {
		s.Err = "连接采集输出超过限制，保留上次结果"
		return s
	}
	if err != nil {
		if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 || out.Len() != 0 || stderr.Len() != 0 {
			s.Err = fmt.Sprintf("连接采集失败：%v %s", err, strings.TrimSpace(stderr.String()))
			return s
		}
	}
	s.Rows, err = Parse(out.Bytes())
	if err != nil {
		s.Err = err.Error()
		return s
	}
	if len(s.Rows) > MaxRows {
		s.Rows = s.Rows[:MaxRows]
		s.Warning = "连接数量超过限制，仅显示前 16384 条"
	}
	if stderr.Len() > 0 {
		s.Warning = strings.TrimSpace(s.Warning + " " + stderr.String())
	}
	// A concrete local binding can identify its interface even when nettop
	// cannot report per-peer counters (for example, a TCP listener).
	interfaces, _ := net.Interfaces()
	byIP := map[string]string{}
	for _, iface := range interfaces {
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err == nil {
				ip := prefix.Addr().Unmap().String()
				if previous, ok := byIP[ip]; ok && previous != iface.Name {
					byIP[ip] = "" // Ambiguous bindings must not guess a route.
				} else if !ok {
					byIP[ip] = iface.Name
				}
			}
		}
	}
	for i := range s.Rows {
		if ip, err := netip.ParseAddr(s.Rows[i].Local.IP); err == nil {
			if ip.Zone() != "" {
				s.Rows[i].Interface = ip.Zone()
			} else {
				s.Rows[i].Interface = byIP[ip.Unmap().String()]
			}
		}
	}
	infos, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		s.Warning = strings.TrimSpace(s.Warning + " 无法核对进程启动时间")
	} else {
		ids := map[int32]int64{}
		for _, p := range infos {
			ids[p.Proc.P_pid] = p.Proc.P_starttime.Sec*1000 + int64(p.Proc.P_starttime.Usec)/1000
		}
		for i := range s.Rows {
			started := ids[s.Rows[i].PID]
			if started <= s.At.UnixMilli() {
				s.Rows[i].Started = started
			}
		}
	}
	return s
}
