// Package ports reads local TCP listeners using macOS's lsof field format.
package ports

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Listener struct {
	PID                                int32
	Started                            int64
	Process, Protocol, Family, Address string
	Port                               int
}

func (l Listener) Key() string {
	return fmt.Sprintf("%d/%d/%s/%s/%s/%d", l.PID, l.Started, l.Protocol, l.Family, l.Address, l.Port)
}
func (l Listener) Endpoint() string { return net.JoinHostPort(l.Address, strconv.Itoa(l.Port)) }
func (l Listener) LocalOnly() bool  { return net.ParseIP(l.Address).IsLoopback() }

type Snapshot struct {
	At        time.Time
	Listeners []Listener
	Err       string
}

// Parse keeps each bound address/family, while collapsing duplicated file descriptors.
func Parse(data []byte) ([]Listener, error) {
	rows := []Listener{}
	seen := map[string]bool{}
	var pid int32
	var command string
	var current Listener
	haveFile := false
	flush := func() {
		if !haveFile || current.Protocol != "TCP" || current.Port <= 0 {
			return
		}
		if key := current.Key(); !seen[key] {
			seen[key] = true
			rows = append(rows, current)
		}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 1 {
			continue
		}
		value := line[1:]
		switch line[0] {
		case 'p':
			flush()
			haveFile = false
			command = ""
			n, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("无法解析端口 PID: %q", value)
			}
			pid = int32(n)
		case 'c':
			command = value
		case 'f':
			flush()
			current = Listener{PID: pid, Process: command}
			haveFile = true
		case 't':
			current.Family = value
		case 'P':
			current.Protocol = value
		case 'n':
			// Split at the last colon: lsof emits *, IPv4 and [IPv6] endpoints.
			i := strings.LastIndexByte(value, ':')
			if i < 0 || strings.Contains(value, "->") {
				continue
			}
			n, err := strconv.Atoi(value[i+1:])
			if err != nil || n < 1 || n > 65535 {
				return nil, fmt.Errorf("无法解析端口地址: %q", value)
			}
			current.Address = strings.Trim(value[:i], "[]")
			current.Port = n
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		return a.Key() < b.Key()
	})
	return rows, nil
}
