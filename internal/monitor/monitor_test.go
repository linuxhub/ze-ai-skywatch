package monitor

import (
	"context"
	"github.com/shirou/gopsutil/v4/cpu"
	"math"
	"os"
	"testing"
	"time"
)

func TestCPUPercent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after float64
		elapsed       time.Duration
		want          float64
		valid         bool
	}{
		{"single core", 1, 3, 2 * time.Second, 100, true},
		{"multiple cores", 1, 6, 2 * time.Second, 250, true},
		{"idle", 1, 1, time.Second, 0, true},
		{"counter reset", 5, 1, time.Second, 0, false},
		{"zero interval", 1, 2, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := CPUPercent(tc.before, tc.after, tc.elapsed)
			if ok != tc.valid || math.Abs(got-tc.want) > 0.001 {
				t.Fatalf("got %v,%v; want %v,%v", got, ok, tc.want, tc.valid)
			}
		})
	}
}
func TestSystemCPU(t *testing.T) {
	p, ok := systemPercent(cpu.TimesStat{User: 10, System: 10, Idle: 80}, cpu.TimesStat{User: 20, System: 20, Idle: 160})
	if !ok || p != 20 {
		t.Fatalf("got %v,%v", p, ok)
	}
	if _, ok = systemPercent(cpu.TimesStat{}, cpu.TimesStat{}); ok {
		t.Fatal("zero delta must be unavailable")
	}
}
func TestLiveCurrentProcess(t *testing.T) {
	s := NewSampler()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	first := s.Sample(ctx)
	if first.CPUReady {
		t.Fatal("first sample needs a baseline")
	}
	second := s.Sample(ctx)
	for _, p := range second.Processes {
		if p.PID == int32(os.Getpid()) {
			if !p.CPUReady || !p.MemoryReadable || p.Memory == 0 {
				t.Fatalf("current process not sampled: %+v", p)
			}
			if _, err := Inspect(ctx, p.Identity); err != nil {
				t.Fatal(err)
			}
			wrong := p.Identity
			wrong.Started++
			if _, err := Inspect(ctx, wrong); err == nil {
				t.Fatal("reused PID identity was accepted")
			}
			return
		}
	}
	t.Fatalf("current process missing; errors: %v", second.Errors)
}

func TestNativeTaskRead(t *testing.T) {
	task, err := readTask(int32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if task.memory == 0 || task.threads < 1 {
		t.Fatalf("invalid task result: %+v", task)
	}
	if _, err := readTask(-1); err == nil {
		t.Fatal("invalid PID returned readable zeroes")
	}
}

func BenchmarkSample(b *testing.B) {
	s := NewSampler()
	ctx := context.Background()
	s.Sample(ctx)
	b.ResetTimer()
	for b.Loop() {
		s.Sample(ctx)
	}
}
