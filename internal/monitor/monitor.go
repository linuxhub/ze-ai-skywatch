// Package monitor samples system and process counters without blocking the UI.
package monitor

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

type Identity struct {
	PID     int32
	Started int64 // Unix milliseconds; PID alone can be reused by the OS.
}

type Process struct {
	Identity
	Name           string
	CPU            float64
	CPUReady       bool
	Memory         uint64 // Resident set, bytes. Not Activity Monitor's memory footprint.
	MemoryReadable bool
}

type Snapshot struct {
	At                      time.Time
	CPU                     float64
	CPUReady                bool
	MemoryUsed, MemoryTotal uint64
	MemoryReady             bool
	ProcessCount            int
	Unreadable              int
	Processes               []Process
	Errors                  []string
}

type counter struct {
	seconds float64
	at      time.Time
	name    string
	namedAt time.Time
}
type Sampler struct {
	previous map[Identity]counter
	system   *cpu.TimesStat
}

func NewSampler() *Sampler { return &Sampler{previous: make(map[Identity]counter)} }

// CPUPercent uses one core as 100%, and can exceed 100 on multicore systems.
func CPUPercent(previous, current float64, elapsed time.Duration) (float64, bool) {
	if elapsed <= 0 || current < previous {
		return 0, false
	}
	v := (current - previous) / elapsed.Seconds() * 100
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func systemPercent(before, after cpu.TimesStat) (float64, bool) {
	busy := func(t cpu.TimesStat) float64 {
		return t.User + t.System + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	}
	db := busy(after) - busy(before)
	dt := db + after.Idle - before.Idle
	if dt <= 0 || db < 0 {
		return 0, false
	}
	return math.Max(0, math.Min(100, db/dt*100)), true
}

// Sample is called serially by the sampling worker. Failed reads remain
// explicitly unavailable; inaccessible processes never become fake zeroes.
func (s *Sampler) Sample(ctx context.Context) Snapshot {
	out := Snapshot{At: time.Now()}
	if times, err := cpu.TimesWithContext(ctx, false); err == nil && len(times) > 0 {
		if s.system != nil {
			out.CPU, out.CPUReady = systemPercent(*s.system, times[0])
		}
		s.system = &times[0]
	} else if err != nil {
		out.Errors = append(out.Errors, "CPU: "+err.Error())
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		out.MemoryUsed, out.MemoryTotal, out.MemoryReady = vm.Used, vm.Total, true
	} else {
		out.Errors = append(out.Errors, "内存: "+err.Error())
	}
	infos, err := enumerateProcesses(ctx)
	if err != nil {
		out.Errors = append(out.Errors, "进程: "+err.Error())
		return out
	}
	out.ProcessCount = len(infos)
	next := make(map[Identity]counter, len(infos))
	for _, row := range infos {
		if ctx.Err() != nil {
			break
		}
		pid := row.PID
		prev, havePrevious := s.previous[row.Identity]
		namedAt := prev.namedAt
		if havePrevious && time.Since(prev.namedAt) < 30*time.Second {
			row.Name = prev.name
		} else {
			// Resolve the full name only for new identities or once every 30 seconds.
			if p, err := process.NewProcessWithContext(ctx, pid); err == nil {
				if name, err := p.NameWithContext(ctx); err == nil && name != "" {
					row.Name = name
				}
			}
			namedAt = time.Now()
		}
		if row.Name == "" {
			row.Name = fmt.Sprintf("进程 %d", pid)
		}
		if task, err := readTask(pid); err == nil && (task.started == 0 || task.started == row.Started) {
			now := time.Now()
			if havePrevious {
				row.CPU, row.CPUReady = CPUPercent(prev.seconds, task.seconds, now.Sub(prev.at))
			}
			next[row.Identity] = counter{seconds: task.seconds, at: now, name: row.Name, namedAt: namedAt}
			row.Memory, row.MemoryReadable = task.memory, true
		} else {
			out.Unreadable++
		}
		out.Processes = append(out.Processes, row)
	}

	s.previous = next
	sort.SliceStable(out.Processes, func(i, j int) bool {
		a, b := out.Processes[i], out.Processes[j]
		if a.CPUReady != b.CPUReady {
			return a.CPUReady
		}
		if a.CPU != b.CPU {
			return a.CPU > b.CPU
		}
		return a.PID < b.PID
	})
	return out
}

type Detail struct {
	Identity
	Name, Path, Command, User string
	Threads                   int32
	Errors                    []string
}

// Inspect checks the identity again instead of trusting a row's cached PID.
func Inspect(ctx context.Context, id Identity) (Detail, error) {
	p, err := process.NewProcessWithContext(ctx, id.PID)
	if err != nil {
		return Detail{}, fmt.Errorf("进程已退出或无法访问: %w", err)
	}
	started, err := p.CreateTimeWithContext(ctx)
	if err != nil {
		return Detail{}, err
	}
	if id.Started != 0 && started != id.Started {
		return Detail{}, fmt.Errorf("进程已退出，PID 已被其他进程使用")
	}
	d := Detail{Identity: Identity{id.PID, started}}
	d.Name, _ = p.NameWithContext(ctx)
	if d.Path, err = p.ExeWithContext(ctx); err != nil {
		d.Errors = append(d.Errors, "可执行文件路径不可读")
	}
	if d.Command, err = p.CmdlineWithContext(ctx); err != nil {
		d.Errors = append(d.Errors, "命令行不可读")
	}
	d.User, _ = p.UsernameWithContext(ctx)
	if task, err := readTask(id.PID); err == nil {
		d.Threads = task.threads
	}
	return d, nil
}
