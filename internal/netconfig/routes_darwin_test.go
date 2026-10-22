package netconfig

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMacRouteTableAbbreviationsAndNeighborFiltering(t *testing.T) {
	rows, err := parseRouteTable(`Routing tables
Internet:
Destination Gateway Flags Netif Expire
default 172.18.13.236 UGScg en8
default 192.168.50.1 UGScIg en0
10.6/24 10.6.0.13 UGSc utun4
192.168.110 10.6.0.13 UGSc utun4
127 127.0.0.1 UCS lo0
172.18.13.236 60:be:b4:2f:10:4c UHLWIir en8 1182
192.168.50.1/32 link#12 UCS en0 !
`, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 6 || rows[2].Destination != "10.6.0.0/24" || rows[3].Destination != "192.168.110.0/24" || rows[4].Destination != "127.0.0.0/8" {
		t.Fatalf("%+v", rows)
	}
	if !rows[1].Scoped || !rows[3].TunnelPolicy() || rows[5].NextHop() != "直连" {
		t.Fatal(rows)
	}
	if _, err := parseRouteTable("not a routing table", false); err == nil {
		t.Fatal("unrecognized output claimed successful empty table")
	}
	if _, err := parseRouteTable("Destination Gateway Flags Netif\nbad-prefix 1.1.1.1 UGS en0", false); err == nil {
		t.Fatal("invalid route was silently discarded")
	}
}

func TestMacIPv6ScopedRouteTable(t *testing.T) {
	rows, err := parseRouteTable(`Destination Gateway Flags Netif Expire
default fe80::%utun0 UGcIg utun0
fe80::%en0/64 link#12 UcI en0
2001:db8:1::/48 fe80::1%utun4 UGS utun4
::1 ::1 UHL lo0
`, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0].Destination != "::/0" || rows[1].Destination != "fe80::/64" || rows[2].Gateway != "fe80::1%utun4" {
		t.Fatal(rows)
	}
	matcher := NewRouteMatcher(RouteTable{At: time.Now(), Rows: rows})
	if match := matcher.Match("2001:db8:1::8"); !match.Found || match.Rule.Interface != "utun4" {
		t.Fatal(match)
	}
}

func TestLiveRoutePredictions(t *testing.T) {
	if os.Getenv("NETCONFIG_LIVE_TEST") != "1" {
		t.Skip("set NETCONFIG_LIVE_TEST=1 to compare system routes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	table := readRoutes(ctx)
	if table.Err != "" {
		t.Fatal(table.Err)
	}
	matcher := NewRouteMatcher(table)
	targets := []string{"1.1.1.1"}
	for _, rule := range table.Rows {
		if rule.TunnelPolicy() {
			prefix, err := macRoutePrefix(rule.Destination, false)
			if err == nil {
				targets = append(targets, prefix.Addr().Next().String())
				break
			}
		}
	}
	for _, target := range targets {
		predicted := matcher.Match(target)
		if !predicted.Found {
			t.Fatalf("%s: %s", target, predicted.Reason)
		}
		output, err := command(ctx, "/sbin/route", "-n", "get", "-inet", target)
		if err != nil {
			t.Fatal(err)
		}
		actual := parseRoute(output)
		if predicted.Rule.Interface != actual.Interface {
			t.Fatalf("%s predicted %s, system chose %s", target, predicted.Rule.Interface, actual.Interface)
		}
		t.Logf("%s → %s via %s", target, predicted.Rule.Interface, predicted.Rule.Destination)
	}
}
