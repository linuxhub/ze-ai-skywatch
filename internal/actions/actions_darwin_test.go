package actions

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/shirou/gopsutil/v4/process"
)

func TestTerminationIdentityAndOwnedChild(t *testing.T) {
	ctx := context.Background()
	if err := Terminate(ctx, monitor.Identity{PID: int32(os.Getpid()), Started: 1}, false); err == nil {
		t.Fatal("self termination allowed")
	}
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer child.Process.Kill()
	p, err := process.NewProcess(int32(child.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	start, err := p.CreateTime()
	if err != nil {
		t.Fatal(err)
	}
	id := monitor.Identity{PID: p.Pid, Started: start + 1}
	if err := Terminate(ctx, id, false); err == nil {
		t.Fatal("stale identity accepted")
	}
	id.Started = start
	if err := Terminate(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("owned child did not exit")
	}
}
