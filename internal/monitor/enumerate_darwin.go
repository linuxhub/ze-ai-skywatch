package monitor

import (
	"context"
	"golang.org/x/sys/unix"
	"strings"
)

func enumerateProcesses(ctx context.Context) ([]Process, error) {
	infos, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	rows := make([]Process, 0, len(infos))
	for _, info := range infos {
		if ctx.Err() != nil {
			break
		}
		rows = append(rows, Process{Identity: Identity{PID: info.Proc.P_pid, Started: info.Proc.P_starttime.Sec*1000 + int64(info.Proc.P_starttime.Usec)/1000}, Name: strings.TrimRight(string(info.Proc.P_comm[:]), "\x00")})
	}
	return rows, nil
}
