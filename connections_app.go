package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/linuxhub/ze-ai-skywatch/internal/network"
)

type connectionsState struct {
	snapshot                                        connections.Snapshot
	traffic                                         network.Snapshot
	trafficErr                                      string
	query, protocol, mode, scope, state, annotation string
	paused, running, automatic, scanning            bool
	cancel                                          context.CancelFunc
	generation                                      uint64
	refresh                                         chan struct{}
	list                                            ui.ListState
	selected                                        int
	selectedKey                                     string
	order                                           ui.SortOrder
	detailOpen                                      bool
	detail                                          connections.Connection
	processOrigin                                   *connections.Connection
	projection                                      connectionsProjection
	rows                                            []connections.Connection
}
type connectionsProjection struct {
	At                                                      time.Time
	Query, Protocol, Mode, Scope, State, Annotation, Column string
	Descending                                              bool
	Notes                                                   uint64
}

func (a *app) initConnections() {
	a.connections = connectionsState{protocol: "全部协议", mode: "全部连接", scope: "全部范围", state: "全部状态", annotation: "全部标注", selected: -1, order: ui.SortOrder{Column: "process"}}
}

// UI-thread lifecycle guard: visiting the page starts an independent collector.
// Leaving, hiding or pausing cancels it and rejects queued results by generation.
func (a *app) syncConnections() {
	n := &a.connections
	visible := a.page == "connections" && a.network.visible
	if n.running && (!visible || (n.automatic && n.paused)) {
		a.stopConnections()
	}
	if visible && !n.paused && !n.running && a.win != nil {
		a.startConnections(true)
	}
}
func (a *app) stopConnections() {
	n := &a.connections
	n.generation++
	if n.cancel != nil {
		n.cancel()
		n.cancel = nil
	}
	n.running, n.scanning = false, false
}
func (a *app) startConnections(automatic bool) {
	if a.win == nil {
		return
	}
	n := &a.connections
	a.stopConnections()
	n.traffic, n.trafficErr = network.Snapshot{}, ""
	ctx, cancel := context.WithCancel(a.ctx)
	n.cancel, n.running, n.automatic, n.scanning = cancel, true, automatic, true
	n.refresh = make(chan struct{}, 1)
	generation, refresh, win := n.generation, n.refresh, a.win
	go func() {
		defer cancel()
		// This page owns its traffic sampler independently of detailed analysis.
		// A manual refresh takes two samples, then tears the child down too.
		trafficDone := make(chan struct{})
		go func() {
			defer close(trafficDone)
			var trafficCtx context.Context
			var stopTraffic context.CancelFunc
			if automatic {
				trafficCtx, stopTraffic = context.WithCancel(ctx)
			} else {
				trafficCtx, stopTraffic = context.WithTimeout(ctx, 8*time.Second)
			}
			defer stopTraffic()
			samples := 0
			err := network.Stream(trafficCtx, func(s network.Snapshot) {
				samples++
				win.Update(func() { a.publishConnectionsTraffic(generation, s, "") })
				if !automatic && samples >= 2 {
					stopTraffic()
				}
			})
			if ctx.Err() == nil && err != nil && (automatic || samples < 2) {
				win.Update(func() {
					a.publishConnectionsTraffic(generation, network.Snapshot{}, "速率采集不可用："+err.Error())
				})
			}
		}()
		defer func() { cancel(); <-trafficDone }()
		collect := func() {
			win.Update(func() {
				if a.connections.generation == generation {
					a.connections.scanning = true
				}
			})
			s := connections.Scan(ctx)
			if ctx.Err() != nil {
				return
			}
			win.Update(func() { a.publishConnections(generation, s) })
		}
		collect()
		if automatic {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for ctx.Err() == nil {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					collect()
				case <-refresh:
					collect()
				}
			}
		} else {
			<-trafficDone
			win.Update(func() {
				if a.connections.generation == generation {
					a.connections.running = false
					a.connections.cancel = nil
				}
			})
		}
	}()
}
func (a *app) publishConnections(generation uint64, s connections.Snapshot) {
	n := &a.connections
	if n.generation != generation {
		return
	}
	if s.Err == "" {
		n.snapshot = connections.WithTraffic(s, n.traffic)
		n.rows = nil
	} else {
		n.snapshot.Err = s.Err
	}
	n.scanning = false
}
func (a *app) publishConnectionsTraffic(generation uint64, s network.Snapshot, err string) {
	n := &a.connections
	if n.generation != generation {
		return
	}
	n.traffic, n.trafficErr = s, err
	n.snapshot = connections.WithTraffic(n.snapshot, s)
	n.rows = nil
}
func (a *app) refreshConnections() {
	if a.page != "connections" {
		return
	}
	if a.connections.running && a.connections.automatic {
		send(a.connections.refresh)
	} else {
		a.startConnections(false)
	}
}

func (a *app) filteredConnections() []connections.Connection {
	n := &a.connections
	key := connectionsProjection{n.snapshot.At, n.query, n.protocol, n.mode, n.scope, n.state, n.annotation, n.order.Column, n.order.Descending, a.ipNotes.revision}
	if n.rows != nil && key == n.projection {
		return n.rows
	}
	q := strings.ToLower(strings.TrimSpace(n.query))
	rows := make([]connections.Connection, 0, len(n.snapshot.Rows))
	for _, row := range n.snapshot.Rows {
		if n.protocol != "全部协议" && row.Protocol != n.protocol {
			continue
		}
		if n.mode == "监听与绑定" && !row.Listening() {
			continue
		}
		if n.mode == "有对端连接" && row.Remote.IP == "" {
			continue
		}
		if n.scope != "全部范围" && row.Scope() != n.scope {
			continue
		}
		if n.state != "全部状态" {
			if n.state == "其他" {
				if row.State == "LISTEN" || row.State == "ESTABLISHED" {
					continue
				}
			} else if row.State != n.state {
				continue
			}
		}
		note := a.ipNote(row.Remote.IP)
		if n.annotation == "已标注" && note.Name == "" {
			continue
		}
		if n.annotation == "未标注" && note.Name != "" {
			continue
		}
		haystack := row.Process + " " + strconv.Itoa(int(row.PID)) + " " + row.Protocol + " " + row.Local.String() + " " + row.Remote.String() + " " + row.Status() + " " + row.Interface + " " + note.Name + " " + note.Group + " " + note.Description
		if q != "" && !strings.Contains(strings.ToLower(haystack), q) {
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		x, y := rows[i], rows[j]
		cmp := 0
		switch n.order.Column {
		case "pid":
			cmp = compare(float64(x.PID), float64(y.PID))
		case "protocol":
			cmp = strings.Compare(x.Protocol, y.Protocol)
		case "local":
			cmp = strings.Compare(x.Local.IP, y.Local.IP)
		case "localPort":
			cmp = compare(float64(portNumber(x.Local.Port)), float64(portNumber(y.Local.Port)))
		case "remote":
			cmp = strings.Compare(x.Remote.IP, y.Remote.IP)
		case "remotePort":
			cmp = compare(float64(portNumber(x.Remote.Port)), float64(portNumber(y.Remote.Port)))
		case "state":
			cmp = strings.Compare(x.Status(), y.Status())
		case "download", "upload":
			if x.Ready != y.Ready {
				return x.Ready // Unknown samples stay below measured rates.
			}
			if n.order.Column == "download" {
				cmp = compare(x.Download, y.Download)
			} else {
				cmp = compare(x.Upload, y.Upload)
			}
		case "interface":
			cmp = strings.Compare(x.Interface, y.Interface)
		case "note":
			cmp = strings.Compare(a.ipNote(x.Remote.IP).Name, a.ipNote(y.Remote.IP).Name)
		default:
			cmp = strings.Compare(strings.ToLower(x.Process), strings.ToLower(y.Process))
		}
		if cmp == 0 {
			return x.Key() < y.Key()
		}
		if n.order.Descending {
			return cmp > 0
		}
		return cmp < 0
	})
	n.projection, n.rows = key, rows
	return rows
}
func portNumber(s string) int { n, _ := strconv.Atoi(s); return n }

func (a *app) socketProcess(row connections.Connection) monitor.Process {
	id := monitor.Identity{PID: row.PID, Started: row.Started}
	for _, p := range a.snapshot.Processes {
		if p.Identity == id {
			return p
		}
	}
	return monitor.Process{Identity: id, Name: row.Process}
}

func (a *app) openSocketProcess(row connections.Connection) {
	if row.PID <= 0 || row.Started <= 0 {
		return
	}
	a.connections.detailOpen = false
	a.openProcess(a.socketProcess(row), nil)
	a.connections.processOrigin = &row
}
func (a *app) selectedSocket() (connections.Connection, bool) {
	for _, row := range a.filteredConnections() {
		if row.Key() == a.connections.selectedKey {
			return row, true
		}
	}
	return connections.Connection{}, false
}
func socketText(row connections.Connection) string {
	return fmt.Sprintf("%s (PID %d)\n%s / %s\n本地 %s\n远端 %s\n状态 %s\n下载 %s\n上传 %s\n网卡 %s", row.Process, row.PID, row.Protocol, row.Family, row.Local, row.Remote, row.Status(), rateText(row.Download, row.Ready), rateText(row.Upload, row.Ready), placeholder(row.Interface))
}
