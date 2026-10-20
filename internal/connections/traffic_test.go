package connections

import (
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/linuxhub/ze-ai-skywatch/internal/network"
)

func TestTrafficOwnedConnectionCancellation(t *testing.T) {
	if os.Getenv("NETWORK_LIVE_TEST") != "1" {
		t.Skip("set NETWORK_LIVE_TEST=1 for owned flow integration")
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
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	readerDone, writerDone := make(chan struct{}), make(chan struct{})
	go func() { defer close(readerDone); _, _ = io.Copy(io.Discard, server) }()
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
	sockets := Scan(ctx)
	if sockets.Err != "" {
		t.Fatal(sockets.Err)
	}
	seen := false
	err = network.Stream(ctx, func(traffic network.Snapshot) {
		for _, row := range WithTraffic(sockets, traffic).Rows {
			if row.PID == int32(os.Getpid()) && row.Local.String() == client.LocalAddr().String() && row.Ready && row.Upload > 0 && row.Interface == "lo0" {
				seen = true
				cancel()
			}
		}
	})
	cancel()
	_ = client.Close()
	_ = server.Close()
	<-readerDone
	<-writerDone
	if !seen {
		t.Fatalf("owned socket traffic / interface missing: %v", err)
	}
}

func TestTrafficJoinRequiresIdentityAndEndpoints(t *testing.T) {
	row := Connection{PID: 42, Started: 123, Protocol: "TCP", Local: Endpoint{"192.168.1.2", "5555"}, Remote: Endpoint{"1.1.1.1", "443"}, Interface: "en0"}
	match := network.Connection{Identity: network.Identity{PID: 42, Started: 123}, Protocol: "TCP", Local: row.Local.String(), Remote: row.Remote.String(), Interface: "utun2", Traffic: network.Traffic{Download: 2048, Upload: 512, Ready: true}}
	for _, kind := range []string{"exact", "recycledPID", "unknownIdentity", "remote", "local", "protocol", "missing", "baseline", "zero"} {
		t.Run(kind, func(t *testing.T) {
			flow := match
			switch kind {
			case "recycledPID":
				flow.Started++
			case "unknownIdentity":
				flow.Started = 0
			case "remote":
				flow.Remote = "1.1.1.1:80"
			case "local":
				flow.Local = "192.168.1.2:5556"
			case "protocol":
				flow.Protocol = "UDP"
			case "baseline":
				flow.Ready = false
			case "zero":
				flow.Download, flow.Upload = 0, 0
			}
			traffic := network.Snapshot{Connections: []network.Connection{flow}}
			if kind == "missing" {
				traffic.Connections = nil
			}
			original := Snapshot{Rows: []Connection{row}}
			got := WithTraffic(original, traffic).Rows[0]
			wantReady := kind == "exact" || kind == "zero"
			if got.Ready != wantReady {
				t.Fatalf("ready: %+v", got)
			}
			if kind == "exact" && (got.Download != 2048 || got.Upload != 512 || got.Interface != "utun2") {
				t.Fatal(got)
			}
			if original.Rows[0].Ready || original.Rows[0].Interface != "en0" {
				t.Fatal("mutated published input")
			}
		})
	}
}
