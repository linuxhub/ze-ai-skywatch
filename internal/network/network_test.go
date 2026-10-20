package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLightBaselineResetAndInterfaceChanges(t *testing.T) {
	var s LightSampler
	at := time.Unix(100, 0)
	first := s.Update(at, []Counter{{"en0", 1000000, 2000000}})
	if first.Ready || first.Received != 0 {
		t.Fatal(first)
	}
	next := s.Update(at.Add(2*time.Second), []Counter{{"en0", 1002000, 2001000}, {"en7", 9000000, 8000000}})
	if !next.Ready || next.Download != 1000 || next.Upload != 500 || next.Received != 2000 {
		t.Fatal(next)
	}
	reset := s.Update(at.Add(4*time.Second), []Counter{{"en0", 5, 8}})
	if reset.Download != 0 || reset.Upload != 0 || reset.Received != 2000 {
		t.Fatal(reset)
	}
}

func TestNettopCSVIPv6UDPAndColumnOrder(t *testing.T) {
	input := ",bytes_out,state,bytes_in,interface,\n" +
		"tcpdump.42,20,,100,,\n" +
		"tcp6 fe80::1%en0:5000<->2001:4860:4860::8888:443,20,Established,100,en0,\n" +
		"udp4 192.168.1.2:5500<->8.8.8.8:53,0,,20,en0,\n" +
		"udp4 192.168.1.2:5500<->8.8.8.8:53,0,,20,en0,\n" +
		"quic4 192.168.1.2:5501<->8.8.8.8:443,0,,20,en0,\n" +
		"tcp4 127.0.0.1:55<->*:*,,,,lo0,\n" +
		",bytes_out,state,bytes_in,interface,\n"
	var frames []rawFrame
	err := readFrames(strings.NewReader(input), func(f rawFrame) error { frames = append(frames, f); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || len(frames[0].Processes) != 1 || len(frames[0].Connections) != 3 {
		t.Fatalf("frames: %+v", frames)
	}
	c := frames[0].Connections[0]
	if frames[0].Connections[1].Received != 40 {
		t.Fatal("duplicate UDP endpoints must aggregate")
	}
	if c.PID != 42 || c.Protocol != "TCP" || c.Remote != "[2001:4860:4860::8888]:443" || c.RemoteIP() != "2001:4860:4860::8888" || c.Received != 100 || c.Sent != 20 {
		t.Fatal(c)
	}
	for _, bad := range []string{",bytes_in,\n", ",bytes_in,bytes_out,\napp.2,1\n", ",bytes_in,bytes_out,\napp.2,oops,2\n"} {
		if err := readFrames(strings.NewReader(bad), func(rawFrame) error { return nil }); err == nil {
			t.Fatalf("accepted invalid CSV %q", bad)
		}
	}
}

func TestTrackerSessionBaselinePIDReuseAndDisappearingFlow(t *testing.T) {
	at := time.Unix(100, 0)
	id := Identity{42, 10}
	tracker := NewTracker()
	frame := func(at time.Time, id Identity, in, out uint64) rawFrame {
		return rawFrame{At: at, Processes: []Process{{Identity: id, Name: "app", Active: true, Traffic: Traffic{Received: in, Sent: out}}}, Connections: []Connection{{Identity: id, Protocol: "TCP", Local: "127.0.0.1:1", Remote: "8.8.8.8:443", Traffic: Traffic{Received: in, Sent: out}}}}
	}
	first := tracker.Update(frame(at, id, 1000, 500))
	if first.Processes[0].Ready || first.Processes[0].Received != 0 {
		t.Fatal(first)
	}
	next := tracker.Update(frame(at.Add(2*time.Second), id, 3000, 1000))
	if next.Processes[0].Download != 1000 || next.Processes[0].Received != 2000 || next.Connections[0].Sent != 500 {
		t.Fatal(next)
	}
	tracker.Update(rawFrame{At: at.Add(4 * time.Second)})
	reappeared := tracker.Update(frame(at.Add(6*time.Second), id, 9000, 2000))
	if reappeared.Processes[0].Ready || reappeared.Processes[0].Received != 2000 {
		t.Fatal(reappeared)
	}
	reused := tracker.Update(frame(at.Add(8*time.Second), Identity{42, 20}, 1, 1))
	for _, p := range reused.Processes {
		if p.Started == 20 && (p.Ready || p.Received != 0) {
			t.Fatal(p)
		}
	}
	reset := tracker.Update(frame(at.Add(10*time.Second), Identity{42, 20}, 0, 0))
	for _, p := range reset.Processes {
		if p.Started == 20 && (p.Download != 0 || p.Upload != 0) {
			t.Fatal(p)
		}
	}
}

func TestBufferedNettopFramesUseSourceTime(t *testing.T) {
	// Delivered all at once, as pipe buffering / a slow metadata lookup does.
	input := "time,,bytes_in,bytes_out,\n09:00:00.123456,app.42,1000,500,\ntime,,bytes_in,bytes_out,\n09:00:02.123456,app.42,3000,1500,\n"
	tracker := NewTracker()
	var snapshots []Snapshot
	err := readFrames(strings.NewReader(input), func(f rawFrame) error {
		if !f.Timed {
			t.Fatal("missing source time")
		}
		for i := range f.Processes {
			f.Processes[i].Started = 10
		}
		snapshots = append(snapshots, tracker.Update(f))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 || snapshots[1].Processes[0].Download != 1000 || snapshots[1].Processes[0].Upload != 500 {
		t.Fatalf("buffering inflated rates: %+v", snapshots)
	}
	ref := time.Date(2026, 10, 20, 23, 59, 59, 0, time.Local)
	next, err := nettopTime("00:00:01.000000", ref)
	if err != nil || next.Sub(ref) != 2*time.Second {
		t.Fatalf("midnight: %v %v", next, err)
	}
}

func TestPublicIPAndAppGrouping(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "100.64.1.2", "::1", "fe80::1", "2001:db8::1", "203.0.113.1", "not an IP"} {
		if PublicIP(ip) {
			t.Fatal(ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "2001:4860:4860::8888"} {
		if !PublicIP(ip) {
			t.Fatal(ip)
		}
	}
	label := labelForPath("Helper", "/Applications/Browser.app/Contents/Frameworks/Helper.app/Contents/MacOS/Helper")
	if label.Name != "Browser" || label.Group != "/Applications/Browser.app" {
		t.Fatal(label)
	}
}

func TestReadPhysicalCounters(t *testing.T) {
	rows, err := ReadCounters()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if !strings.HasPrefix(row.Name, "en") {
			t.Fatal(row)
		}
	}
}

// Opt-in live test is bounded and uses only sockets owned by this test. Run in
// a normal macOS process: the execution sandbox blocks NStat and local sockets.
func TestStreamOwnedConnectionCancellation(t *testing.T) {
	if os.Getenv("NETWORK_LIVE_TEST") != "1" {
		t.Skip("set NETWORK_LIVE_TEST=1 for bounded live nettop test")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	done := make(chan struct{})
	go func() { defer close(done); _, _ = io.Copy(io.Discard, server) }()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = client.SetWriteDeadline(time.Now().Add(time.Second))
				if _, err := client.Write(make([]byte, 65536)); err != nil {
					return
				}
			}
		}
	}()
	seen := false
	err = Stream(ctx, func(s Snapshot) {
		for _, c := range s.Connections {
			if c.PID == int32(os.Getpid()) && c.Local == client.LocalAddr().String() && c.Remote == client.RemoteAddr().String() && c.Ready && c.Upload > 0 {
				seen = true
				cancel()
				return
			}
		}
	})
	cancel()
	_ = client.Close()
	_ = server.Close()
	<-done
	<-writerDone
	if !seen {
		t.Fatalf("owned flow missing / no rate: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal(fmt.Sprintf("stream did not cancel: %v", err))
	}
}

func BenchmarkPhysicalCounters(b *testing.B) {
	for b.Loop() {
		if _, err := ReadCounters(); err != nil {
			b.Fatal(err)
		}
	}
}
