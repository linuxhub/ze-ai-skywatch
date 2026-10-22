package netconfig

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMacServicesAndIndependentIPDNSModes(t *testing.T) {
	services := parseServices("An asterisk (*) denotes that a network service is disabled.\n(1) Office LAN\n(Hardware Port: USB LAN, Device: en8)\n(*) Wi-Fi\n(Hardware Port: Wi-Fi, Device: en0)\n(3) VPN\n(Hardware Port: VPN, Device: )")
	if services["en8"].name != "Office LAN" || !services["en0"].disabled || len(services) != 2 {
		t.Fatal(services)
	}
	adapter := Adapter{IPv4Method: Unknown, IPv6Method: Unknown}
	applyMacInfo(&adapter, "DHCP Configuration\nIP address: 192.168.1.8\nRouter: 192.168.1.1\nIPv6: Automatic")
	adapter.DNS, adapter.DNSMode = parseManualDNS("1.1.1.1\n2606:4700:4700::1111")
	if adapter.IPv4Method != "DHCP 自动获取" || adapter.DNSMode != "手动配置" || len(adapter.DNS) != 2 || len(adapter.Gateway4) != 1 {
		t.Fatal(adapter)
	}
	applyMacInfo(&adapter, "Manual Configuration\nRouter: none\nIPv6: Off")
	if adapter.IPv4Method != "手动固定 IP" || adapter.IPv6Method != "已关闭" {
		t.Fatal(adapter)
	}
	if _, mode := parseManualDNS("There aren't any DNS Servers set on Wi-Fi."); mode != "自动／未手动指定" {
		t.Fatal(mode)
	}
	if _, mode := parseManualDNS("unrecognized output"); mode != Unknown {
		t.Fatal(mode)
	}
}

func TestMacDNSKeepsScopedAndSupplementalResolversSeparate(t *testing.T) {
	global, scoped := parseResolvers(`DNS configuration
resolver #1
  nameserver[0] : 1.1.1.1
  if_index : 8 (utun3)
resolver #2
  domain : corporate.example
  nameserver[0] : 10.0.0.53
  if_index : 8 (utun3)
DNS configuration (for scoped queries)
resolver #1
  nameserver[0] : 192.168.1.1
  if_index : 10 (en8)
resolver #2
  nameserver[0] : fe80::1%en0
  if_index : 12 (en0)
DNS configuration (for service-specific queries)
resolver #1
  nameserver[0] : 10.9.0.53
`)
	if !reflect.DeepEqual(global, []string{"1.1.1.1"}) || !reflect.DeepEqual(scoped["en8"], []string{"192.168.1.1"}) || !reflect.DeepEqual(scoped["en0"], []string{"fe80::1%en0"}) {
		t.Fatalf("global=%v scoped=%v", global, scoped)
	}
}

func TestMacProxyDoesNotConfuseScopedSettingsWithSystemDefault(t *testing.T) {
	proxies, exceptions := parseMacProxies(`<dictionary> {
  HTTPEnable : 0
  HTTPSEnable : 1
  HTTPSProxy : 127.0.0.1
  HTTPSPort : 7890
  ExceptionsList : <array> {
    0 : localhost
    1 : *.local
  }
  ProxyAutoConfigEnable : 1
  ProxyAutoConfigURLString : https://user:pass@example.com/a.pac?token=secret
  __SCOPED__ : <dictionary> {
    en0 : <dictionary> {
      HTTPEnable : 1
      HTTPProxy : 10.0.0.9
    }
  }
}`)
	if proxies[0].Enabled || !proxies[1].Enabled || proxies[1].Endpoint() != "127.0.0.1:7890" || len(exceptions) != 2 {
		t.Fatalf("%+v %v", proxies, exceptions)
	}
	if proxies[3].Endpoint() != "https://example.com/a.pac" {
		t.Fatal(proxies[3])
	}
	unknown, _ := parseMacProxies("<dictionary> {\n}")
	if (Snapshot{Proxies: unknown}).ProxyStatus() != Unknown {
		t.Fatal("empty dictionary claimed disabled")
	}
}

func TestMacIPv6Route(t *testing.T) {
	route := parseRoute("route to: default\n gateway: fe80::1%en0\n interface: en0\n")
	if route.Interface != "en0" || route.Gateway != "fe80::1%en0" {
		t.Fatal(route)
	}
}

func TestMacCancelledScanDoesNotPublishPartialSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if snapshot := Scan(ctx); snapshot.Err == "" {
		t.Fatal("canceled scan reported success")
	}
}

func TestLiveNetconfig(t *testing.T) {
	if os.Getenv("NETCONFIG_LIVE_TEST") != "1" {
		t.Skip("set NETCONFIG_LIVE_TEST=1 to inspect this machine's network configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snapshot := Scan(ctx)
	if snapshot.Err != "" || len(snapshot.Adapters) == 0 {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	if snapshot.Default4.Interface == "" || len(snapshot.DNS) == 0 || snapshot.ProxyStatus() == Unknown {
		t.Fatalf("incomplete network configuration: %s", snapshot.Text())
	}
	if strings.Contains(snapshot.Text(), "password") {
		t.Fatal("unexpected credential")
	}
	t.Logf("read %d adapters; default=%s; DNS=%d; proxy=%s; warnings=%v", len(snapshot.Adapters), snapshot.Default4.Interface, len(snapshot.DNS), snapshot.ProxyStatus(), snapshot.Warnings)
}
