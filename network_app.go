package main

import (
	"context"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/network"
)

type networkPoint struct {
	At               time.Time
	Download, Upload float64
}
type networkState struct {
	light                                                     network.LightSnapshot
	history                                                   []networkPoint
	enabled, visible                                          bool
	cancel                                                    context.CancelFunc
	generation                                                uint64
	started                                                   time.Time
	snapshot                                                  network.Snapshot
	err                                                       string
	group                                                     bool
	query, order, filter, selectedProcess, selectedConnection string
	processList, connectionList                               ui.ListState
	processSelected, connectionSelected                       int
	detailOpen                                                bool
	detail                                                    network.Connection
	geo                                                       map[string]network.Location
	geoCancel                                                 context.CancelFunc
	geoGeneration                                             uint64
	geoBusy                                                   bool
	geoPaused                                                 bool
	geoRevision                                               uint64
	processKey, connectionKey, mapKey                         networkProjectionKey
	processRows                                               []networkRow
	connectionRows                                            []network.Connection
	mapRows                                                   []mapDestination
	mapViewportSize, mapWindowSize                            [2]float32
	mapLayoutNotices                                          [4]bool
}
type networkProjectionKey struct {
	At                             time.Time
	Query, Order, Filter, Selected string
	Group                          bool
	Geo                            uint64
	Notes                          uint64
}

func (a *app) networkProjection() networkProjectionKey {
	n := &a.network
	return networkProjectionKey{At: n.snapshot.At, Query: n.query, Order: n.order, Filter: n.filter, Selected: n.selectedConnection, Group: n.group, Geo: n.geoRevision, Notes: a.ipNotes.revision}
}
func (a *app) initNetwork() {
	a.network = networkState{visible: true, group: true, order: "合计速度", processSelected: -1, connectionSelected: -1, geo: map[string]network.Location{}}
}

func (a *app) startNetworkLight() {
	go func() {
		var sampler network.LightSampler
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			if a.ctx.Err() != nil {
				return
			}
			s := sampler.Sample()
			if a.ctx.Err() != nil {
				return
			}
			a.win.Update(func() {
				if s.Err != "" {
					a.network.light.Err = s.Err
					a.network.light.Ready = false
					return
				}
				a.network.light = s
				if s.Ready {
					a.network.history = append(a.network.history, networkPoint{s.At, s.Download, s.Upload})
					if len(a.network.history) > 90 {
						a.network.history = a.network.history[len(a.network.history)-90:]
					}
				}
			})
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Called on the UI thread. The generation check rejects queued snapshots and
// lookup results even when cancellation races with Window.Update.
func (a *app) setNetworkAnalysis(enabled bool) {
	n := &a.network
	if runtime.GOOS == "windows" && enabled {
		n.enabled = false
		return
	}
	n.generation++
	if n.cancel != nil {
		n.cancel()
		n.cancel = nil
	}
	a.stopNetworkGeo()
	n.enabled = enabled
	n.snapshot = network.Snapshot{}
	n.err = ""
	n.geo = map[string]network.Location{}
	n.geoPaused = false
	n.geoRevision++
	n.processRows = nil
	n.connectionRows = nil
	n.mapRows = nil
	n.detailOpen = false
	n.filter = ""
	n.selectedProcess = ""
	n.selectedConnection = ""
	n.processSelected = -1
	n.connectionSelected = -1
	n.processList = ui.ListState{}
	n.connectionList = ui.ListState{}
	if !enabled {
		n.started = time.Time{}
		return
	}
	n.started = time.Now()
	if a.win == nil {
		return
	} // Headless UI tests do not start external collectors.
	ctx, cancel := context.WithCancel(a.ctx)
	n.cancel = cancel
	generation := n.generation
	go func() {
		err := network.Stream(ctx, func(s network.Snapshot) {
			if ctx.Err() != nil {
				return
			}
			a.win.Update(func() {
				if a.network.generation != generation || !a.network.enabled {
					return
				}
				a.network.snapshot = s
				a.scheduleNetworkGeo()
			})
		})
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			if a.network.generation != generation {
				return
			}
			a.setNetworkAnalysis(false)
			if err != nil {
				a.network.err = err.Error()
			}
		})
	}()
}

func (a *app) stopNetworkGeo() {
	n := &a.network
	n.geoGeneration++
	if n.geoCancel != nil {
		n.geoCancel()
		n.geoCancel = nil
	}
	n.geoBusy = false
}
func (a *app) changeNetworkGeo() {
	a.stopNetworkGeo()
	// Disabling also releases the enrichment cache and allows explicit retry.
	a.network.geo = map[string]network.Location{}
	a.network.geoPaused = false
	a.network.geoRevision++
	a.persist()
	a.scheduleNetworkGeo()
}

// A serial queue, at most 200 unique destinations per analysis session. Cache
// errors as well as successes; no unbounded retries or per-frame HTTP calls.
func (a *app) scheduleNetworkGeo() {
	n := &a.network
	if !n.enabled || !a.config.NetworkGeo || n.geoBusy || n.geoPaused || a.win == nil || len(n.geo) >= 200 {
		return
	}
	var ip string
	for _, c := range n.snapshot.Connections {
		candidate := c.RemoteIP()
		if _, ok := n.geo[candidate]; !ok && network.PublicIP(candidate) {
			ip = candidate
			break
		}
	}
	if ip == "" {
		return
	}
	n.geoBusy = true
	n.geo[ip] = network.Location{IP: ip, Err: "查询中"}
	ctx, cancel := context.WithCancel(a.ctx)
	n.geoCancel = cancel
	generation, session := n.geoGeneration, n.generation
	go func() {
		defer cancel()
		client := &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		location := network.Lookup(ctx, client, ip)
		// Pace enrichment; a full map never spawns a burst of lookup requests.
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		a.win.Update(func() {
			if a.network.generation != session || a.network.geoGeneration != generation || !a.network.enabled || !a.config.NetworkGeo {
				return
			}
			a.network.geoBusy = false
			a.network.geoCancel = nil
			a.network.geo[ip] = location
			a.network.geoPaused = location.Limited
			a.network.geoRevision++
			a.scheduleNetworkGeo()
		})
	}()
}

type networkRow struct {
	Key, Name   string
	Processes   []network.Identity
	Traffic     network.Traffic
	Connections int
	Active      bool
}

func (a *app) networkProcesses() []networkRow {
	key := a.networkProjection()
	key.Filter = ""
	key.Selected = ""
	if key == a.network.processKey && a.network.processRows != nil {
		return a.network.processRows
	}
	groups := map[string]*networkRow{}
	query := strings.ToLower(strings.TrimSpace(a.network.query))
	matchedIDs := map[network.Identity]bool{}
	if query != "" {
		for _, c := range a.network.snapshot.Connections {
			if strings.Contains(strings.ToLower(c.Local+" "+c.Remote+" "+a.network.geo[c.RemoteIP()].Organization+" "+a.noteSearch(c.RemoteIP())), query) {
				matchedIDs[c.Identity] = true
			}
		}
	}
	matchedGroups := map[string]bool{}
	for _, p := range a.network.snapshot.Processes {
		key, name := p.Identity.Key(), p.Name
		if a.network.group {
			key = p.Group
			name = p.App
			if key == "" {
				key = p.Name
			}
			if name == "" {
				name = p.Name
			}
		}
		row := groups[key]
		if row == nil {
			row = &networkRow{Key: key, Name: name}
			groups[key] = row
		}
		row.Processes = append(row.Processes, p.Identity)
		row.Traffic.Download += p.Download
		row.Traffic.Upload += p.Upload
		row.Traffic.Received += p.Received
		row.Traffic.Sent += p.Sent
		row.Traffic.Ready = row.Traffic.Ready || p.Ready
		row.Connections += p.Connections
		row.Active = row.Active || p.Active
		if query == "" || matchedIDs[p.Identity] || strings.Contains(strings.ToLower(p.Name+" "+p.Identity.Key()+" "+name+" "+key), query) {
			matchedGroups[key] = true
		}
	}
	rows := make([]networkRow, 0, len(groups))
	for _, row := range groups {
		if matchedGroups[row.Key] {
			rows = append(rows, *row)
		}
	}
	score := func(t network.Traffic) float64 {
		switch a.network.order {
		case "下载速度":
			return t.Download
		case "上传速度":
			return t.Upload
		case "累计流量":
			return float64(t.Received + t.Sent)
		default:
			return t.Download + t.Upload
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		x, y := score(rows[i].Traffic), score(rows[j].Traffic)
		if x != y {
			return x > y
		}
		return rows[i].Key < rows[j].Key
	})
	a.network.processKey = key
	a.network.processRows = rows
	return rows
}
func (a *app) networkConnections() []network.Connection {
	key := a.networkProjection()
	key.Order = ""
	key.Selected = ""
	if key == a.network.connectionKey && a.network.connectionRows != nil {
		return a.network.connectionRows
	}
	rows := make([]network.Connection, 0, len(a.network.snapshot.Connections))
	query := strings.ToLower(strings.TrimSpace(a.network.query))
	ids := map[network.Identity]bool{}
	queryIDs := map[network.Identity]bool{}
	if query != "" {
		for _, p := range a.network.snapshot.Processes {
			if strings.Contains(strings.ToLower(p.Name+" "+p.App+" "+p.Identity.Key()), query) {
				queryIDs[p.Identity] = true
			}
		}
	}
	if a.network.filter != "" {
		for _, p := range a.network.snapshot.Processes {
			key := p.Identity.Key()
			if a.network.group {
				key = p.Group
				if key == "" {
					key = p.Name
				}
			}
			if key == a.network.filter {
				ids[p.Identity] = true
			}
		}
	}
	for _, c := range a.network.snapshot.Connections {
		if a.network.filter != "" && !ids[c.Identity] {
			continue
		}
		if query != "" && !queryIDs[c.Identity] && !strings.Contains(strings.ToLower(c.Process+" "+c.Identity.Key()+" "+c.Local+" "+c.Remote+" "+a.network.geo[c.RemoteIP()].Organization+" "+a.noteSearch(c.RemoteIP())), query) {
			continue
		}
		rows = append(rows, c)
	}
	a.network.connectionKey = key
	a.network.connectionRows = rows
	return rows
}
