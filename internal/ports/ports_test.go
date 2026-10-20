package ports

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
)

func TestParseFields(t *testing.T) {
	data := []byte("p42\ncMy App\nf4\ntIPv4\nPTCP\nn127.0.0.1:8080\nf5\ntIPv4\nPTCP\nn127.0.0.1:8080\nf6\ntIPv6\nPTCP\nn[::1]:8080\np90\ncserver\nf7\ntIPv6\nPTCP\nn*:443\n")
	rows, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 endpoints, got %+v", rows)
	}
	if rows[0].Port != 443 || rows[0].PID != 90 {
		t.Fatalf("wrong grouping/order: %+v", rows)
	}
	if rows[1].Process != "My App" || !rows[1].LocalOnly() || !rows[2].LocalOnly() {
		t.Fatalf("lost name or loopback: %+v", rows)
	}
	if rows[2].Address != "::1" || rows[2].Endpoint() != "[::1]:8080" {
		t.Fatal(rows[2])
	}
	if rows[0].LocalOnly() {
		t.Fatal("wildcard is not loopback")
	}
}
func TestParseErrorsAndEmpty(t *testing.T) {
	if rows, err := Parse(nil); err != nil || len(rows) != 0 {
		t.Fatalf("empty: %v %v", rows, err)
	}
	for _, data := range []string{"pnope\n", "p1\ncapp\nf1\nPTCP\nn*:oops\n", "p1\nf1\nPTCP\nn*:70000\n"} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Fatalf("expected error for %q", data)
		}
	}
}
func TestScanOwnedListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	s := Scan(context.Background())
	if s.Err != "" {
		t.Fatal(s.Err)
	}
	for _, row := range s.Listeners {
		if row.Port == port && row.PID == int32(os.Getpid()) && row.Address == "127.0.0.1" {
			return
		}
	}
	t.Fatal(fmt.Sprintf("owned listener %d/%d absent from %d results", os.Getpid(), port, len(s.Listeners)))
}
