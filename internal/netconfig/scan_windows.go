package netconfig

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const windowsScript = `
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$warnings = @()
$adapters = @(Get-NetAdapter -IncludeHidden)
$addresses = @(Get-NetIPAddress)
$interfaces = @(Get-NetIPInterface)
$routes = @(Get-NetRoute | Where-Object { $_.DestinationPrefix -in @('0.0.0.0/0','::/0') })
$dns = @(Get-DnsClientServerAddress)
$rows = @()
foreach ($adapter in $adapters) {
  $index = $adapter.ifIndex
  $ipv4 = $interfaces | Where-Object { $_.InterfaceIndex -eq $index -and $_.AddressFamily -eq 2 } | Select-Object -First 1
  $ipv6 = $interfaces | Where-Object { $_.InterfaceIndex -eq $index -and $_.AddressFamily -eq 23 } | Select-Object -First 1
  $method4 = '无法确定'
  if ($null -ne $ipv4) { if ([int]$ipv4.Dhcp -eq 1) { $method4 = 'DHCP 自动获取' } else { $method4 = 'DHCP 已关闭（地址来源见各 IP）' } }
  $method6 = '无法确定'
  if ($null -ne $ipv6) { $method6 = '自动／手动配置（来源见各 IP）' }
  $ipRows = @()
  foreach ($address in @($addresses | Where-Object { $_.InterfaceIndex -eq $index })) {
    $origin = '无法确定'
    switch ([int]$address.PrefixOrigin) { 1 { $origin = '手动配置' }; 2 { $origin = '系统地址' }; 3 { $origin = 'DHCP' }; 4 { $origin = '路由通告／SLAAC' } }
    $ipRows += @{ IP = [string]$address.IPAddress; Prefix = [int]$address.PrefixLength; Method = $origin }
  }
  $v4Addresses = @($addresses | Where-Object { $_.InterfaceIndex -eq $index -and $_.AddressFamily -eq 2 })
  if ($null -ne $ipv4 -and [int]$ipv4.Dhcp -eq 0 -and $v4Addresses.Count -gt 0 -and @($v4Addresses | Where-Object { [int]$_.PrefixOrigin -ne 1 }).Count -eq 0) { $method4 = '手动固定 IP' }
  $mode4 = '无法确定'; $mode6 = '无法确定'
  foreach ($family in @('Tcpip','Tcpip6')) {
    try {
      $registry = Get-ItemProperty -LiteralPath ('HKLM:\SYSTEM\CurrentControlSet\Services\' + $family + '\Parameters\Interfaces\' + ([guid]$adapter.InterfaceGuid).ToString('B'))
      $mode = '自动／未手动指定'
      if (-not [string]::IsNullOrWhiteSpace([string]$registry.NameServer)) { $mode = '手动配置' }
      if ($family -eq 'Tcpip') { $mode4 = $mode } else { $mode6 = $mode }
    } catch {}
  }
  $status = '未连接'
  if ($adapter.Status -eq 'Up') { $status = '已连接' }
  if ($adapter.Status -eq 'Disabled') { $status = '已禁用' }
  $kind = [string]$adapter.InterfaceDescription
  $rows += @{
    Index = [int]$index; Name = [string]$adapter.Name; Service = [string]$adapter.Name; Kind = $kind
    Status = $status; Up = ($adapter.Status -eq 'Up'); Virtual = (-not $adapter.HardwareInterface)
    MAC = [string]$adapter.MacAddress; MTU = [int]$ipv4.NlMtu; Addresses = $ipRows
    IPv4Method = $method4; IPv6Method = $method6; DNSMode = ('IPv4：' + $mode4 + '；IPv6：' + $mode6)
    DNS = @($dns | Where-Object { $_.InterfaceIndex -eq $index } | ForEach-Object { $_.ServerAddresses } | Select-Object -Unique)
    Gateway4 = @($routes | Where-Object { $_.InterfaceIndex -eq $index -and $_.DestinationPrefix -eq '0.0.0.0/0' } | ForEach-Object { [string]$_.NextHop } | Select-Object -Unique)
    Gateway6 = @($routes | Where-Object { $_.InterfaceIndex -eq $index -and $_.DestinationPrefix -eq '::/0' } | ForEach-Object { [string]$_.NextHop } | Select-Object -Unique)
  }
}
$default4 = @{}; $default6 = @{}
foreach ($prefix in @('0.0.0.0/0','::/0')) {
  $candidates = @()
  foreach ($route in @($routes | Where-Object { $_.DestinationPrefix -eq $prefix })) {
    $index = $route.InterfaceIndex
    $family = $route.AddressFamily
    $iface = $interfaces | Where-Object { $_.InterfaceIndex -eq $index -and $_.AddressFamily -eq $family } | Select-Object -First 1
    if ($null -ne $iface -and [int]$iface.ConnectionState -eq 1) { $candidates += @{ Interface = [string]$iface.InterfaceAlias; Gateway = [string]$route.NextHop; Cost = ([int]$route.RouteMetric + [int]$iface.InterfaceMetric) } }
  }
  $selected = $candidates | Sort-Object Cost | Select-Object -First 1
  if ($null -ne $selected) {
    $entry = @{ Interface = $selected.Interface; Gateway = $selected.Gateway }
    if ($prefix -eq '0.0.0.0/0') { $default4 = $entry } else { $default6 = $entry }
  }
}
$proxies = @(); $exceptions = @(); $settings = $null
try { $settings = Get-ItemProperty -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' } catch { $warnings += '当前用户系统代理配置不可读' }
$mapping = @{}
if ($null -ne $settings -and $settings.ProxyServer) {
  foreach ($part in ([string]$settings.ProxyServer -split ';')) {
    $pair = $part.Trim() -split '=',2
    if ($pair.Count -eq 2) { $mapping[$pair[0].ToLower()] = $pair[1] } else { $mapping['http'] = $part.Trim(); $mapping['https'] = $part.Trim() }
  }
}
foreach ($kind in @('http','https','socks')) {
  $server = ''; $port = 0
  if ($mapping.ContainsKey($kind)) {
    $endpoint = $mapping[$kind]
    if ($endpoint -notmatch '^[a-zA-Z][a-zA-Z0-9+.-]*://') { $endpoint = 'http://' + $endpoint }
    $uri = $null
    if ([Uri]::TryCreate($endpoint, [UriKind]::Absolute, [ref]$uri)) { $server = $uri.DnsSafeHost; $port = $uri.Port }
  }
  $proxies += @{ Kind = $kind.ToUpper(); Known = ($null -ne $settings); Enabled = ($null -ne $settings -and [int]$settings.ProxyEnable -eq 1 -and $mapping.ContainsKey($kind)); Server = $server; Port = $port }
}
$autoKnown = $false; $autoEnabled = $false; $pacEnabled = $false
try {
  $connection = Get-ItemProperty -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings\Connections'
  $bytes = $connection.DefaultConnectionSettings
  if ($null -ne $bytes -and $bytes.Length -gt 11) { $autoKnown = $true; $autoEnabled = (($bytes[8] -band 8) -ne 0); $pacEnabled = (($bytes[8] -band 4) -ne 0) }
} catch {}
$proxies += @{ Kind = 'PAC'; Known = $autoKnown; Enabled = $pacEnabled; URL = [string]$settings.AutoConfigURL }
$proxies += @{ Kind = '自动发现'; Known = $autoKnown; Enabled = $autoEnabled }
if ($null -ne $settings -and $settings.ProxyOverride) { $exceptions = @([string]$settings.ProxyOverride -split ';') }
@{ Adapters = $rows; Default4 = $default4; Default6 = $default6; Proxies = $proxies; Exceptions = $exceptions; Warnings = $warnings } | ConvertTo-Json -Depth 8 -Compress
`

func Scan(ctx context.Context) Snapshot {
	snapshot := baseSnapshot()
	if snapshot.Err != "" {
		return snapshot
	}
	executable := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	output, err := exec.CommandContext(ctx, executable, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", windowsScript).Output()
	if err != nil {
		snapshot.Err = "读取 Windows 网络配置失败：" + err.Error()
		return snapshot
	}
	var collected Snapshot
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(output), "\ufeff")), &collected); err != nil {
		snapshot.Err = "Windows 网络配置解析失败：" + err.Error()
		return snapshot
	}
	for _, adapter := range collected.Adapters {
		found := false
		for index := range snapshot.Adapters {
			if snapshot.Adapters[index].Index == adapter.Index {
				snapshot.Adapters[index] = adapter
				found = true
				break
			}
		}
		if !found {
			snapshot.Adapters = append(snapshot.Adapters, adapter)
		}
	}
	snapshot.Default4, snapshot.Default6 = collected.Default4, collected.Default6
	snapshot.Proxies, snapshot.Exceptions, snapshot.Warnings = collected.Proxies, collected.Exceptions, collected.Warnings
	snapshot.ProxyScope = "Windows 当前用户系统代理（WinINET）；WinHTTP 与应用自设代理未覆盖"
	snapshot.Routing = readRoutes(ctx)
	for index := range snapshot.Proxies {
		snapshot.Proxies[index].URL = safeURL(snapshot.Proxies[index].URL)
	}
	for _, adapter := range snapshot.Adapters {
		if adapter.Name == snapshot.Default4.Interface || adapter.Name == snapshot.Default6.Interface {
			for _, server := range adapter.DNS {
				snapshot.DNS = appendUnique(snapshot.DNS, server)
			}
		}
	}
	sortAdapters(&snapshot)
	return snapshot
}
