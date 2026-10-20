package connections

import (
	"strings"

	"github.com/linuxhub/ze-ai-skywatch/internal/network"
)

// WithTraffic joins only verified process identities and exact socket endpoints.
// Missing counters remain unknown; process totals must never be assigned to a
// socket. Copy the rows because a published snapshot may still be in use.
func WithTraffic(s Snapshot, traffic network.Snapshot) Snapshot {
	index := make(map[string]network.Connection, len(traffic.Connections))
	for _, row := range traffic.Connections {
		if row.Started != 0 {
			index[row.Key()] = row
		}
	}
	s.Rows = append([]Connection(nil), s.Rows...)
	for i := range s.Rows {
		row := &s.Rows[i]
		row.Traffic = network.Traffic{}
		if row.Started == 0 || row.Remote.IP == "" {
			continue
		}
		key := (network.Connection{Identity: network.Identity{PID: row.PID, Started: row.Started}, Protocol: row.Protocol, Local: row.Local.String(), Remote: row.Remote.String()}).Key()
		if match, ok := index[key]; ok {
			row.Traffic = match.Traffic
			if iface := strings.TrimSpace(match.Interface); iface != "" {
				row.Interface = iface
			}
		}
	}
	return s
}
