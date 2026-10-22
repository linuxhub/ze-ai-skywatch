package appidentity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/shirou/gopsutil/v4/process"
)

type Info struct {
	ProcessName     string `json:"process_name,omitempty"`
	Executable      string `json:"executable,omitempty"`
	AppName         string `json:"app_name_declared,omitempty"`
	AppPath         string `json:"app_path,omitempty"`
	BundleID        string `json:"bundle_id_declared,omitempty"`
	Version         string `json:"version_declared,omitempty"`
	DeclaredCompany string `json:"company_declared,omitempty"`
	Signer          string `json:"signer_declared,omitempty"`
	TeamID          string `json:"team_id,omitempty"`
	Signature       string `json:"disk_signature_check"`
	Parent          string `json:"parent_process,omitempty"`
	Warning         string `json:"warning,omitempty"`
}

func Collect(ctx context.Context, identity monitor.Identity) (Info, error) {
	if identity.PID <= 0 || identity.Started <= 0 {
		return Info{}, errors.New("未确认进程身份，仅分析连接快照")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	proc, err := process.NewProcessWithContext(ctx, identity.PID)
	if err != nil || !sameProcess(ctx, proc, identity.Started) {
		return Info{}, errors.New("进程已结束、身份变化或权限不足，仅分析连接快照")
	}
	info := Info{Signature: "未知"}
	info.ProcessName, _ = proc.NameWithContext(ctx)
	info.Executable, _ = proc.ExeWithContext(ctx)
	if info.Executable == "" {
		info.Warning = "无法读取可执行文件路径，不能确认应用与签名"
	} else {
		before, statErr := os.Stat(info.Executable)
		inspectPlatform(ctx, &info)
		after, afterErr := os.Stat(info.Executable)
		if statErr != nil || afterErr != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			info = Info{ProcessName: info.ProcessName, Signature: "未知", Warning: "文件在读取期间变化或不可访问，已丢弃文件身份信息"}
		}
	}
	if parentID, err := proc.PpidWithContext(ctx); err == nil && parentID > 0 {
		if parent, err := process.NewProcessWithContext(ctx, parentID); err == nil {
			started, _ := parent.CreateTimeWithContext(ctx)
			name, _ := parent.NameWithContext(ctx)
			if started > 0 && sameProcess(ctx, parent, started) {
				info.Parent = name
			}
		}
	}
	if !sameProcess(ctx, proc, identity.Started) {
		return Info{}, errors.New("读取期间进程已结束或身份变化，已丢弃身份信息")
	}
	return info, nil
}

func sameProcess(ctx context.Context, proc *process.Process, started int64) bool {
	if proc == nil || ctx.Err() != nil {
		return false
	}
	fresh, err := process.NewProcessWithContext(ctx, proc.Pid)
	if err != nil {
		return false
	}
	current, err := fresh.CreateTimeWithContext(ctx)
	return err == nil && current == started
}

type boundedOutput struct{ bytes.Buffer }

func (output *boundedOutput) Write(data []byte) (int, error) {
	if output.Len()+len(data) > 256*1024 {
		return 0, errors.New("output limit")
	}
	return output.Buffer.Write(data)
}

func command(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = input
	var output boundedOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	return output.Bytes(), err
}
