package appidentity

import (
	"context"
	"os"
	"testing"

	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/shirou/gopsutil/v4/process"
)

func TestRejectUnverifiedIdentity(t *testing.T) {
	for _, identity := range []monitor.Identity{{}, {PID: int32(os.Getpid())}, {PID: int32(os.Getpid()), Started: 1}} {
		info, err := Collect(context.Background(), identity)
		if err == nil || info.Executable != "" || info.Signer != "" {
			t.Fatal("unverified identity returned executable evidence", info, err)
		}
	}
}

func TestCollectCurrentProcess(t *testing.T) {
	if os.Getenv("ZE_TEST_IDENTITY") == "" {
		t.Skip("requires local process inspection")
	}
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	started, err := proc.CreateTime()
	if err != nil {
		t.Fatal(err)
	}
	info, err := Collect(context.Background(), monitor.Identity{PID: int32(os.Getpid()), Started: started})
	if err != nil || info.Executable == "" || info.ProcessName == "" || info.Signature == "" {
		t.Fatalf("missing local identity evidence: %+v; %v", info, err)
	}
}
