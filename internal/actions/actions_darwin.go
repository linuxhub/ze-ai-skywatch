package actions

import (
	"context"
	"fmt"
	"os"

	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"
)

// Terminate never elevates privileges. The UI must obtain confirmation first.
func Terminate(ctx context.Context, id monitor.Identity, force bool) error {
	if id.PID <= 1 || id.PID == int32(os.Getpid()) || id.Started == 0 {
		return fmt.Errorf("不能结束系统关键进程、本应用或身份未确认的进程")
	}
	if _, err := monitor.Inspect(ctx, id); err != nil {
		return err
	}
	p, err := process.NewProcessWithContext(ctx, id.PID)
	if err != nil {
		return err
	}
	uids, err := p.UidsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("无法确认进程所属用户: %w", err)
	}
	owned := false
	for _, uid := range uids {
		if uid == uint32(os.Geteuid()) {
			owned = true
		}
	}
	if !owned {
		return fmt.Errorf("当前用户无权停止此进程")
	}
	// Re-read the start time immediately before sending the signal.
	p, err = process.NewProcessWithContext(ctx, id.PID)
	if err != nil {
		return err
	}
	started, err := p.CreateTimeWithContext(ctx)
	if err != nil {
		return err
	}
	if started != id.Started {
		return fmt.Errorf("进程已变化，请重新打开详情")
	}
	signal := unix.SIGTERM
	if force {
		signal = unix.SIGKILL
	}
	if err = unix.Kill(int(id.PID), signal); err != nil {
		return fmt.Errorf("停止进程失败: %w", err)
	}
	return nil
}
