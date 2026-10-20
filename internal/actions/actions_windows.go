package actions

import (
	"context"
	"fmt"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"golang.org/x/sys/windows"
	"os"
)

// Keep one process handle for identity, ownership and termination. A recycled
// PID cannot redirect the termination to a different process after validation.
func Terminate(ctx context.Context, id monitor.Identity, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id.PID <= 4 || id.PID == int32(os.Getpid()) || id.Started == 0 {
		return fmt.Errorf("不能结束系统关键进程、本应用或身份未确认的进程")
	}
	if !force {
		return fmt.Errorf("Windows 版仅支持确认后强制结束进程")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(id.PID))
	if err != nil {
		return fmt.Errorf("无法打开进程: %w", err)
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return err
	}
	if created.Nanoseconds()/1e6 != id.Started {
		return fmt.Errorf("进程已变化，请重新打开详情")
	}
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return fmt.Errorf("无法确认进程所属用户: %w", err)
	}
	defer token.Close()
	owner, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if !owner.User.Sid.Equals(current.User.Sid) {
		return fmt.Errorf("只允许停止当前用户所属的进程")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := windows.TerminateProcess(h, 1); err != nil {
		return fmt.Errorf("停止进程失败: %w", err)
	}
	return nil
}
