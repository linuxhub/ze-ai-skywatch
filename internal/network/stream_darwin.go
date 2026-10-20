package network

import (
	"context"
	"fmt"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"
	"os/exec"
	"strings"
	"time"
)

// Stream uses one long-lived child, not one shell command per refresh. A frame
// is complete when the next CSV header arrives (one sample of publication lag).
// CommandContext kills nettop on cancellation and Wait always reaps the child.
func Stream(ctx context.Context, publish func(Snapshot)) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/nettop", "-n", "-x", "-L", "0", "-s", "2", "-J", "time,interface,state,bytes_in,bytes_out")
	cmd.WaitDelay = 2 * time.Second
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		return err
	}
	tracker := NewTracker()
	labels := map[Identity]appLabel{}
	err = readFrames(pipe, func(frame rawFrame) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !frame.Timed {
			return fmt.Errorf("nettop 未提供采样时间，无法可靠计算速率")
		}
		// Use PID + birth time; never attribute a recycled PID to the old app.
		infos, e := unix.SysctlKinfoProcSlice("kern.proc.all")
		if e != nil {
			return fmt.Errorf("无法核对联网进程身份: %w", e)
		}
		ids := map[int32]int64{}
		for _, info := range infos {
			ids[info.Proc.P_pid] = info.Proc.P_starttime.Sec*1000 + int64(info.Proc.P_starttime.Usec)/1000
		}
		for i := range frame.Processes {
			p := &frame.Processes[i]
			p.Started = ids[p.PID]
			if p.Started > frame.At.UnixMilli() {
				p.Started = 0
			}
			if p.Started == 0 {
				continue
			}
			label, ok := labels[p.Identity]
			if !ok {
				label = appLabel{Name: p.Name, Group: p.Identity.Key()}
				if proc, e := process.NewProcessWithContext(ctx, p.PID); e == nil {
					if path, e := proc.ExeWithContext(ctx); e == nil {
						label = labelForPath(p.Name, path)
					}
				}
				if len(labels) < 4096 {
					labels[p.Identity] = label
				}
			}
			p.App = label.Name
			p.Group = label.Group
		}
		for i := range frame.Connections {
			c := &frame.Connections[i]
			c.Started = ids[c.PID]
			if c.Started > frame.At.UnixMilli() {
				c.Started = 0
			}
		}
		publish(tracker.Update(frame))
		return nil
	})
	if err != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	if waitErr != nil {
		return fmt.Errorf("详细网络采集失败: %w %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return fmt.Errorf("详细网络采集意外结束 %s", strings.TrimSpace(stderr.String()))
}
