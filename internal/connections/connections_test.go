package connections

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestParseSocketKinds(t *testing.T) {
	data := "p42\ncMy App\nf1\ntIPv4\nPTCP\nn*:8080\nTST=LISTEN\nf2\ntIPv4\nPTCP\nn*:8080\nTST=LISTEN\nf3\ntIPv4\nPTCP\nn192.168.1.2:53400->121.43.212.142:443\nTST=ESTABLISHED\nTQR=0\nf4\ntIPv6\nPTCP\nn[fe80::1%en0]:4000->[fe80::2%en0]:443\nTST=CLOSE_WAIT\np43\ncresolver\nf5\ntIPv6\nPUDP\nn*:5353\nf6\ntIPv4\nPUDP\nn127.0.0.1:6000->127.0.0.1:9997\nf7\ntIPv4\nPUDP\nn*:*\n"
	rows, err := Parse([]byte(data))
	if err != nil || len(rows) != 6 {
		t.Fatalf("parse: %v %+v", err, rows)
	}
	statuses := map[string]int{}
	for _, r := range rows {
		statuses[r.Status()]++
		if r.PID == 42 && r.Process != "My App" {
			t.Fatal("process name lost")
		}
		if r.State == "ESTABLISHED" && (r.Local.Port != "53400" || r.Remote.IP != "121.43.212.142" || r.Listening()) {
			t.Fatal(r)
		}
		if r.State == "CLOSE_WAIT" && r.Remote.String() != "[fe80::2%en0]:443" {
			t.Fatal(r)
		}
		if r.Status() == "有对端" && r.Scope() != "本机回环" {
			t.Fatal(r)
		}
	}
	for _, s := range []string{"LISTEN", "ESTABLISHED", "CLOSE_WAIT", "已绑定", "有对端", "未绑定"} {
		if statuses[s] != 1 {
			t.Fatal(statuses)
		}
	}
}
func TestParseRejectsInvalidAndAllowsEmpty(t *testing.T) {
	if rows, err := Parse(nil); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	for _, s := range []string{"pbad\n", "p1\nf1\nPTCP\nn*:65536\n", "p1\nf1\nPUDP\nn*:oops\n", "p1\nf1\nPTCP\nnno-ip:443\n", "p1\nf1\nPTCP\nn127.0.0.1:80->broken\n"} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}
func TestScanOwnedTCPAndUDP(t *testing.T) {
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
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	peer, err := net.Dial("udp4", udp.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	s := Scan(context.Background())
	if s.Err != "" {
		t.Fatal(s.Err)
	}
	want := map[string]bool{
		fmt.Sprint(listener.Addr().(*net.TCPAddr).Port) + "/LISTEN":         false,
		fmt.Sprint(client.LocalAddr().(*net.TCPAddr).Port) + "/ESTABLISHED": false,
		fmt.Sprint(udp.LocalAddr().(*net.UDPAddr).Port) + "/已绑定":            false,
		fmt.Sprint(peer.LocalAddr().(*net.UDPAddr).Port) + "/有对端":           false,
	}
	for _, r := range s.Rows {
		if r.PID != int32(os.Getpid()) {
			continue
		}
		if r.Local.IP == "127.0.0.1" && r.Interface != "lo0" {
			t.Fatalf("loopback interface missing: %+v", r)
		}
		key := r.Local.Port + "/" + r.Status()
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("missing %s: %+v", key, s)
		}
	}
}
func TestCancelledScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if s := Scan(ctx); s.Err == "" {
		t.Fatal("cancelled scan reported success")
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancel did not stop promptly")
	}
}
