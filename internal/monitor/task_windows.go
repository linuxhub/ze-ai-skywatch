package monitor

import (
	"context"
	"fmt"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

type taskInfo struct {
	seconds float64
	memory  uint64
	threads int32
	started int64
}

func enumerateProcesses(ctx context.Context) ([]Process, error) {
	entries, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]Process, 0, len(entries))
	for _, p := range entries {
		if ctx.Err() != nil {
			break
		}
		started, _ := p.CreateTimeWithContext(ctx)
		name, _ := p.NameWithContext(ctx)
		rows = append(rows, Process{Identity: Identity{PID: p.Pid, Started: started}, Name: name})
	}
	return rows, nil
}

func readTask(pid int32) (taskInfo, error) {
	if pid <= 0 {
		return taskInfo{}, fmt.Errorf("进程不可读")
	}
	p, err := process.NewProcess(pid)
	if err != nil {
		return taskInfo{}, err
	}
	started, err := p.CreateTime()
	if err != nil {
		return taskInfo{}, err
	}
	times, err := p.Times()
	if err != nil {
		return taskInfo{}, err
	}
	memory, err := p.MemoryInfo()
	if err != nil {
		return taskInfo{}, err
	}
	threads, _ := p.NumThreads()
	// Reopen the PID after reading all fields so a recycled PID is never published.
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return taskInfo{}, err
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return taskInfo{}, err
	}
	if created.Nanoseconds()/1e6 != started {
		return taskInfo{}, fmt.Errorf("进程身份已变化")
	}
	return taskInfo{seconds: times.User + times.System, memory: memory.RSS, threads: threads, started: started}, nil
}
