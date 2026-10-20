package ports

import (
	"bytes"
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os/exec"
	"strings"
	"time"
)

func Scan(ctx context.Context) Snapshot {
	s := Snapshot{At: time.Now()}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-FpcfnPt")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		s.Err = "端口扫描超时或已取消"
		return s
	}
	// lsof exits 1 when no file matches. Errors with diagnostics are not empty results.
	if err != nil {
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || len(out) != 0 || stderr.Len() != 0 {
			s.Err = fmt.Sprintf("端口扫描失败: %v %s", err, strings.TrimSpace(stderr.String()))
			return s
		}
	}
	s.Listeners, err = Parse(out)
	if err != nil {
		s.Err = err.Error()
	}
	// Carry the process identity with the scan instead of associating an old
	// listener with a newly reused PID when its details are opened later.
	if infos, err := unix.SysctlKinfoProcSlice("kern.proc.all"); err == nil {
		identities := make(map[int32]int64, len(infos))
		for _, info := range infos {
			identities[info.Proc.P_pid] = info.Proc.P_starttime.Sec*1000 + int64(info.Proc.P_starttime.Usec)/1000
		}
		for i := range s.Listeners {
			s.Listeners[i].Started = identities[s.Listeners[i].PID]
		}
	}
	if stderr.Len() > 0 {
		s.Err = "端口扫描提示: " + strings.TrimSpace(stderr.String())
	}
	return s
}
