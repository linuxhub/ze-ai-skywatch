package ports

import (
	"context"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
	"sort"
	"strconv"
)

func Scan(ctx context.Context) Snapshot {
	sockets := connections.Scan(ctx)
	s := Snapshot{At: sockets.At, Err: sockets.Err}
	// Port snapshots have no separate warning field: preserve previous rows if
	// one of the native socket tables was unavailable rather than undercounting.
	if s.Err == "" && sockets.Warning != "" {
		s.Err = sockets.Warning
	}
	if s.Err != "" {
		return s
	}
	for _, row := range sockets.Rows {
		if row.Protocol != "TCP" || row.State != "LISTEN" {
			continue
		}
		port, err := strconv.Atoi(row.Local.Port)
		if err != nil {
			continue
		}
		s.Listeners = append(s.Listeners, Listener{PID: row.PID, Started: row.Started, Process: row.Process, Protocol: row.Protocol, Family: row.Family, Address: row.Local.IP, Port: port})
	}
	sort.Slice(s.Listeners, func(i, j int) bool {
		if s.Listeners[i].Port != s.Listeners[j].Port {
			return s.Listeners[i].Port < s.Listeners[j].Port
		}
		return s.Listeners[i].Key() < s.Listeners[j].Key()
	})
	return s
}
