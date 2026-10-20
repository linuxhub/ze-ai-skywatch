package network

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Identity struct {
	PID     int32
	Started int64
}

func (id Identity) Key() string { return fmt.Sprintf("%d/%d", id.PID, id.Started) }

type Traffic struct {
	Download, Upload float64
	Received, Sent   uint64
	Ready            bool
}
type Process struct {
	Identity
	Name, App, Group string
	Traffic
	Connections int
	Active      bool
}
type Connection struct {
	Identity
	Process, Protocol, Local, Remote, Interface, State string
	Traffic
}

func (c Connection) Key() string {
	return c.Identity.Key() + "/" + c.Protocol + "/" + c.Local + "/" + c.Remote
}
func (c Connection) RemoteIP() string {
	host, _, err := net.SplitHostPort(c.Remote)
	if err != nil {
		return ""
	}
	if i := strings.LastIndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	return host
}
func (c Connection) Scope() string {
	ip := net.ParseIP(c.RemoteIP())
	if ip == nil {
		return "未知"
	}
	if ip.IsLoopback() {
		return "本机回环 / 可能为本地代理"
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return "局域网"
	}
	if !PublicIP(c.RemoteIP()) {
		return "非公网 / 保留地址"
	}
	return "公网"
}

type Snapshot struct {
	At          time.Time
	Processes   []Process
	Connections []Connection
	Dropped     int
}
type rawFrame struct {
	At          time.Time
	Timed       bool
	Processes   []Process
	Connections []Connection
	Dropped     int
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 4096 {
		_, _ = b.Buffer.Write(p[:min(n, 4096-b.Len())])
	}
	return n, nil
}

type appLabel struct{ Name, Group string }

func labelForPath(fallback, path string) appLabel {
	// The outer .app groups helper processes with their owning application.
	if i := strings.Index(path, ".app/"); i >= 0 {
		root := path[:i+4]
		j := strings.LastIndexByte(root, '/')
		return appLabel{Name: strings.TrimSuffix(root[j+1:], ".app"), Group: root}
	}
	return appLabel{Name: fallback, Group: path}
}

func readFrames(r io.Reader, publish func(rawFrame) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var frame rawFrame
	var columns map[string]int
	var current *Process
	var connectionIndex map[string]int
	var nameColumn int
	var haveTime bool
	var reference time.Time
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		reader := csv.NewReader(strings.NewReader(line))
		reader.FieldsPerRecord = -1
		fields, err := reader.Read()
		if err != nil {
			return fmt.Errorf("nettop CSV 无法解析: %w", err)
		}
		if len(fields) < 2 {
			return fmt.Errorf("nettop CSV 字段不完整")
		}
		if fields[0] == "" || fields[0] == "time" {
			next := map[string]int{}
			for i, name := range fields {
				next[strings.TrimSpace(name)] = i
			}
			if _, ok := next["bytes_in"]; !ok {
				return fmt.Errorf("nettop 缺少 bytes_in 字段")
			}
			if _, ok := next["bytes_out"]; !ok {
				return fmt.Errorf("nettop 缺少 bytes_out 字段")
			}
			if !frame.At.IsZero() {
				if err = publish(frame); err != nil {
					return err
				}
				reference = frame.At
			}
			timed := fields[0] == "time"
			frame = rawFrame{At: time.Now(), Timed: timed}
			nameColumn = 0
			if timed {
				nameColumn = 1
			}
			haveTime = false
			columns = next
			current = nil
			connectionIndex = map[string]int{}
			continue
		}
		if columns == nil {
			return fmt.Errorf("nettop 未提供支持的 CSV 表头")
		}
		if len(fields) <= max(columns["bytes_in"], columns["bytes_out"]) {
			return fmt.Errorf("nettop CSV 行被截断")
		}
		if frame.Timed && !haveTime {
			if reference.IsZero() {
				reference = time.Now()
			}
			at, err := nettopTime(fields[0], reference)
			if err != nil {
				return fmt.Errorf("nettop 采样时间无效: %w", err)
			}
			frame.At = at
			haveTime = true
		}
		get := func(key string) string {
			if i, ok := columns[key]; ok && i < len(fields) {
				return strings.TrimSpace(fields[i])
			}
			return ""
		}
		incoming, outgoing := get("bytes_in"), get("bytes_out")
		var in, out uint64
		if incoming != "" {
			in, err = strconv.ParseUint(incoming, 10, 64)
			if err != nil {
				return fmt.Errorf("nettop 接收字节无效: %w", err)
			}
		}
		if outgoing != "" {
			out, err = strconv.ParseUint(outgoing, 10, 64)
			if err != nil {
				return fmt.Errorf("nettop 发送字节无效: %w", err)
			}
		}
		name := strings.TrimSpace(fields[nameColumn])
		if strings.HasPrefix(name, "tcp") || strings.HasPrefix(name, "udp") || strings.HasPrefix(name, "quic") {
			// Only protocol tokens followed by a space are connection rows. An app
			// named tcpdump.123 is still a process.
			prefix, endpoint, ok := strings.Cut(name, " ")
			if ok && (prefix == "tcp4" || prefix == "tcp6" || prefix == "udp4" || prefix == "udp6" || prefix == "quic4" || prefix == "quic6") {
				if current == nil {
					continue
				}
				local, remote, ok := strings.Cut(strings.TrimSpace(endpoint), "<->")
				if !ok {
					return fmt.Errorf("nettop 连接地址无法解析")
				}
				// Listener / unconnected UDP has no peer and no per-peer traffic.
				if remote == "*:*" || incoming == "" || outgoing == "" {
					continue
				}
				connection := Connection{Identity: current.Identity, Process: current.Name, Protocol: strings.ToUpper(prefix[:len(prefix)-1]), Local: normalizeEndpoint(local), Remote: normalizeEndpoint(remote), Interface: get("interface"), State: get("state"), Traffic: Traffic{Received: in, Sent: out}}
				if index, ok := connectionIndex[connection.Key()]; ok {
					// UDP SO_REUSEPORT can expose several sockets with the same
					// endpoints. Aggregate them before assigning UI row identity.
					frame.Connections[index].Received += in
					frame.Connections[index].Sent += out
					continue
				}
				if len(frame.Connections) >= 8192 {
					frame.Dropped++
					continue
				}
				connectionIndex[connection.Key()] = len(frame.Connections)
				frame.Connections = append(frame.Connections, connection)
				current.Connections++
				continue
			}
		}
		i := strings.LastIndexByte(name, '.')
		if i < 1 {
			return fmt.Errorf("nettop 进程标识无法解析: %q", name)
		}
		pid, err := strconv.ParseInt(name[i+1:], 10, 32)
		if err != nil || pid < 0 {
			return fmt.Errorf("nettop PID 无效: %q", name)
		}
		if incoming == "" || outgoing == "" {
			current = nil
			frame.Dropped++
			continue
		}
		if len(frame.Processes) >= 4096 {
			current = nil
			frame.Dropped++
			continue
		}
		frame.Processes = append(frame.Processes, Process{Identity: Identity{PID: int32(pid)}, Name: name[:i], Traffic: Traffic{Received: in, Sent: out}, Active: true})
		current = &frame.Processes[len(frame.Processes)-1]
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !frame.At.IsZero() {
		return publish(frame)
	}
	return nil
}

// Use the producer's timestamp, never the time a buffered CSV row is read.
// Date is absent from nettop's clock; choose the nearest day to the last sample
// (or now for the first frame), including a midnight transition.
func nettopTime(value string, reference time.Time) (time.Time, error) {
	clock, err := time.Parse("15:04:05.999999999", value)
	if err != nil {
		return time.Time{}, err
	}
	at := time.Date(reference.Year(), reference.Month(), reference.Day(), clock.Hour(), clock.Minute(), clock.Second(), clock.Nanosecond(), reference.Location())
	if at.Before(reference.Add(-12 * time.Hour)) {
		at = at.AddDate(0, 0, 1)
	}
	if at.After(reference.Add(12 * time.Hour)) {
		at = at.AddDate(0, 0, -1)
	}
	return at, nil
}

func normalizeEndpoint(s string) string {
	// nettop prints IPv6 without brackets on some versions; split at last colon.
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return s
	}
	return net.JoinHostPort(strings.Trim(s[:i], "[]"), s[i+1:])
}

type tracked struct {
	in, out uint64
	traffic Traffic
}
type Tracker struct {
	at          time.Time
	previous    map[Identity]tracked
	connections map[string]tracked
	processes   map[Identity]Process
}

func NewTracker() *Tracker {
	return &Tracker{previous: map[Identity]tracked{}, connections: map[string]tracked{}, processes: map[Identity]Process{}}
}
func trafficDelta(before tracked, in, out uint64, dt float64, known bool) Traffic {
	t := before.traffic
	t.Download = 0
	t.Upload = 0
	t.Ready = known && dt > 0 && in >= before.in && out >= before.out
	if t.Ready {
		if in >= before.in {
			n := in - before.in
			t.Received += n
			t.Download = float64(n) / dt
		}
		if out >= before.out {
			n := out - before.out
			t.Sent += n
			t.Upload = float64(n) / dt
		}
	}
	return t
}
func (t *Tracker) Update(f rawFrame) Snapshot {
	out := Snapshot{At: f.At, Dropped: f.Dropped}
	dt := f.At.Sub(t.at).Seconds()
	for id, p := range t.processes {
		p.Active = false
		p.Download = 0
		p.Upload = 0
		p.Connections = 0
		t.processes[id] = p
	}
	for _, p := range f.Processes {
		if p.Started == 0 {
			continue
		}
		prev, ok := t.previous[p.Identity]
		if !ok {
			prev.traffic = t.processes[p.Identity].Traffic
		}
		traffic := trafficDelta(prev, p.Received, p.Sent, dt, ok && !t.at.IsZero())
		t.previous[p.Identity] = tracked{p.Received, p.Sent, traffic}
		p.Traffic = traffic
		if len(t.processes) < 4096 {
			t.processes[p.Identity] = p
		} else if _, ok := t.processes[p.Identity]; ok {
			t.processes[p.Identity] = p
		} else {
			out.Dropped++
		}
	}
	next := map[string]tracked{}
	for _, c := range f.Connections {
		if c.Started == 0 {
			continue
		}
		key := c.Key()
		prev, ok := t.connections[key]
		traffic := trafficDelta(prev, c.Received, c.Sent, dt, ok && !t.at.IsZero())
		next[key] = tracked{c.Received, c.Sent, traffic}
		c.Traffic = traffic
		out.Connections = append(out.Connections, c)
	}
	// Bound both identity history and live connection history to the UI limits.
	for id := range t.previous {
		if p, ok := t.processes[id]; !ok || !p.Active {
			delete(t.previous, id)
		}
	}
	for _, p := range t.processes {
		out.Processes = append(out.Processes, p)
	}
	sort.Slice(out.Processes, func(i, j int) bool {
		a, b := out.Processes[i], out.Processes[j]
		if a.Download+a.Upload != b.Download+b.Upload {
			return a.Download+a.Upload > b.Download+b.Upload
		}
		return a.Identity.Key() < b.Identity.Key()
	})
	sort.Slice(out.Connections, func(i, j int) bool {
		a, b := out.Connections[i], out.Connections[j]
		if a.Download+a.Upload != b.Download+b.Upload {
			return a.Download+a.Upload > b.Download+b.Upload
		}
		return a.Key() < b.Key()
	})
	t.connections = next
	t.at = f.At
	return out
}
