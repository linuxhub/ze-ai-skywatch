package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/linuxhub/ze-ai-skywatch/internal/ai"
	"github.com/linuxhub/ze-ai-skywatch/internal/appidentity"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
	"github.com/linuxhub/ze-ai-skywatch/internal/ipnotes"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/linuxhub/ze-ai-skywatch/internal/netconfig"
)

type connectionEvidence struct {
	Socket               connections.Connection
	SampledAt, RoutesAt  time.Time
	CapturedAt, ConfigAt time.Time
	Route                netconfig.RouteMatch
	Proxy                string
	Note                 ipnotes.Note
	Identity             appidentity.Info
	IdentityError        string
	IdentityAt           time.Time
	CollectionWarnings   []string
}

type connectionAnalysisState struct {
	key                    string
	tab                    int
	sequence               uint64
	evidence               connectionEvidence
	prepared, preparing    bool
	loading, preview       bool
	redactIPs, includeNote bool
	inspectCancel, cancel  context.CancelFunc
	text, err, source      string
	payload                string
	report                 ai.ConnectionReport
	structured             bool
}

func (a *app) cancelConnectionAnalysis() {
	state := &a.connectionAI
	if state.cancel != nil {
		state.cancel()
		state.cancel = nil
	}
	state.sequence++
	state.loading = false
}

func (a *app) syncConnectionAnalysis() {
	state := &a.connectionAI
	if !a.connections.detailOpen {
		if state.key != "" {
			a.cancelConnectionAnalysis()
			if state.inspectCancel != nil {
				state.inspectCancel()
			}
			*state = connectionAnalysisState{sequence: state.sequence}
		}
		return
	}
	if state.key != a.connections.detail.Key() {
		a.cancelConnectionAnalysis()
		if state.inspectCancel != nil {
			state.inspectCancel()
		}
		row := a.connections.detail
		*state = connectionAnalysisState{key: row.Key(), sequence: state.sequence, redactIPs: true,
			evidence: connectionEvidence{Socket: row, SampledAt: a.connections.snapshot.At, RoutesAt: a.netconfig.snapshot.Routing.At,
				CapturedAt: time.Now(), ConfigAt: a.netconfig.snapshot.At, Route: a.matchRoute(row.Remote.IP),
				Proxy: a.netconfig.snapshot.ProxyStatus(), Note: a.ipNote(row.Remote.IP)}}
		for _, warning := range []string{a.connections.snapshot.Err, a.connections.snapshot.Warning, a.netconfig.snapshot.Err, a.netconfig.snapshot.Routing.Err} {
			if warning != "" {
				state.evidence.CollectionWarnings = append(state.evidence.CollectionWarnings, warning)
			}
		}
		if a.netconfig.snapshot.Err != "" {
			state.evidence.Proxy = "无法确认（配置采集失败）"
		}
	}
	if state.tab != 2 || state.prepared || state.preparing || a.win == nil {
		return
	}
	state.preparing = true
	ctx, cancel := context.WithCancel(a.ctx)
	state.inspectCancel = cancel
	key, sequence := state.key, state.sequence
	identity := monitor.Identity{PID: state.evidence.Socket.PID, Started: state.evidence.Socket.Started}
	win := a.win
	go func() {
		defer cancel()
		info, err := appidentity.Collect(ctx, identity)
		win.Update(func() {
			current := &a.connectionAI
			if !a.connectionResultCurrent(key, sequence) {
				return
			}
			current.prepared, current.preparing, current.inspectCancel = true, false, nil
			current.evidence.Identity, current.evidence.IdentityAt = info, time.Now()
			if err != nil {
				current.evidence.IdentityError = err.Error()
			}
		})
	}()
}

var evidenceAddressPattern = regexp.MustCompile(`[0-9a-fA-F:.]+(?:%[a-zA-Z0-9_.-]+)?`)

func connectionPayload(evidence connectionEvidence, redactIPs, includeNote bool) string {
	row, route := evidence.Socket, evidence.Route
	identity := evidence.Identity
	identity.Executable, identity.AppPath = redactUserPath(identity.Executable), redactUserPath(identity.AppPath)
	routeData := map[string]any{"found": route.Found, "reason": route.Reason, "sampled_at": evidence.RoutesAt}
	if route.Found {
		routeData["destination"] = route.Rule.Destination
		routeData["gateway"] = route.Rule.NextHop()
		routeData["interface"] = route.Rule.Interface
		routeData["path_kind"] = route.Rule.Path()
		routeData["reject"] = route.Rule.Reject
	}
	data := map[string]any{
		"os": platformName(), "captured_at": evidence.CapturedAt, "socket_sampled_at": evidence.SampledAt,
		"process":              map[string]any{"name": row.Process, "pid": row.PID, "started_unix_ms": row.Started},
		"application_evidence": identity, "identity_error": evidence.IdentityError, "identity_sampled_at": evidence.IdentityAt,
		"connection": map[string]any{"local_ip": row.Local.IP, "local_port": row.Local.Port, "remote_ip": row.Remote.IP,
			"remote_port": row.Remote.Port, "protocol": row.Protocol, "family": row.Family, "state": row.Status(),
			"scope": row.Scope(), "interface_associated_or_bound": row.Interface, "listening_or_bound": row.Listening()},
		"route_inference":     routeData,
		"system_proxy_config": map[string]any{"status": evidence.Proxy, "sampled_at": evidence.ConfigAt, "used_by_this_socket": "无法确认"},
		"ip_redacted":         redactIPs,
		"collection_warnings": evidence.CollectionWarnings,
		"limitations":         "固定连接快照；本地与远端不是请求发起方向。应用元数据和签名声明并非发布者证明；签名仅检查磁盘文件，不证明进程内存或安全。路由非抓包结果，隧道不能确认VPN客户端。未提供命令行、环境、请求内容、域名、文件内容、其他连接或完整路由表。",
	}
	if includeNote {
		data["user_annotation_unverified"] = map[string]string{"name": evidence.Note.Name, "group": evidence.Note.Group, "description": evidence.Note.Description}
	}
	encoded, _ := json.MarshalIndent(data, "", "  ")
	var safeData any
	json.Unmarshal(encoded, &safeData)
	encoded, _ = json.MarshalIndent(redactEvidencePaths(safeData), "", "  ")
	payload := string(encoded)
	if !redactIPs {
		return payload
	}
	aliases := make(map[netip.Addr]string)
	return evidenceAddressPattern.ReplaceAllStringFunc(payload, func(value string) string {
		address, err := netip.ParseAddr(value)
		if err != nil {
			return value
		}
		address = address.Unmap()
		if alias, ok := aliases[address]; ok {
			return alias
		}
		category := (connections.Connection{Remote: connections.Endpoint{IP: address.String()}}).Scope()
		alias := fmt.Sprintf("IP-%d（%s）", len(aliases)+1, category)
		aliases[address] = alias
		return alias
	})
}

func redactEvidencePaths(value any) any {
	switch typed := value.(type) {
	case string:
		return redactUserPath(typed)
	case map[string]any:
		for key, item := range typed {
			typed[key] = redactEvidencePaths(item)
		}
	case []any:
		for index, item := range typed {
			typed[index] = redactEvidencePaths(item)
		}
	}
	return value
}

func (a *app) startConnectionAnalysis() {
	state := &a.connectionAI
	if !a.connections.detailOpen || state.preparing || state.loading {
		return
	}
	provider, ok := a.activeProvider()
	if !ok {
		state.err = "请先在设置中配置默认模型服务"
		return
	}
	if err := provider.Validate(); err != nil {
		state.err = err.Error()
		return
	}
	if a.win == nil {
		return
	}
	a.cancelConnectionAnalysis()
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	state.cancel, state.loading, state.preview = cancel, true, false
	state.text, state.err, state.source, state.structured = "", "", "", false
	state.report = ai.ConnectionReport{}
	state.payload = connectionPayload(state.evidence, state.redactIPs, state.includeNote)
	payload, key, sequence := state.payload, state.key, state.sequence
	win := a.win
	go func() {
		defer cancel()
		text, model, err := requestConnectionReport(ctx, provider, payload)
		report, parseErr := ai.ParseConnectionReport(text)
		win.Update(func() {
			current := &a.connectionAI
			if !a.connectionResultCurrent(key, sequence) {
				return
			}
			current.loading, current.cancel = false, nil
			if err != nil {
				current.err = err.Error()
				return
			}
			current.text, current.report, current.structured = text, report, parseErr == nil
			current.source = provider.Name + " · " + model + " · " + time.Now().Format("15:04:05")
		})
	}()
}

func (a *app) connectionResultCurrent(key string, sequence uint64) bool {
	return a.connections.detailOpen && a.connections.detail.Key() == key && a.connectionAI.key == key && a.connectionAI.sequence == sequence
}

func requestConnectionReport(ctx context.Context, provider ai.Provider, payload string) (string, string, error) {
	client := ai.NewClient()
	model, err := client.ResolveModel(ctx, provider)
	if err != nil {
		return "", "", err
	}
	text, err := client.Complete(ctx, provider, model, ai.ConnectionPrompt, "请解释此连接快照 JSON：\n"+payload)
	if len(text) > 96*1024 {
		return "", model, fmt.Errorf("报告超出显示上限，请重试")
	}
	return text, model, err
}

func (a *app) connectionReportText() string {
	state := &a.connectionAI
	body := state.text
	if state.structured {
		report := state.report
		body = fmt.Sprintf("摘要\n%s\n\n应用身份\n应用：%s\n公司／发布者：%s\n角色：%s\n依据：%s\n\n连接去向\n%s\n\n网络路径与 VPN\n%s",
			report.Summary, report.Application.Name, report.Application.Company, report.Application.Role, report.Application.Basis, report.Connection, report.Route)
		for _, section := range []struct {
			title string
			items []string
		}{{"关键发现", report.Findings}, {"仍无法确认", report.Unknowns}, {"可选下一步", report.Suggestions}} {
			body += "\n\n" + section.title
			for _, item := range section.items {
				body += "\n- " + item
			}
		}
	}
	return strings.Join([]string{"AI 连接分析 · 模型生成，需结合本机证据", state.source,
		"连接采集：" + sinceLabel(state.evidence.SampledAt), "路由采集：" + sinceLabel(state.evidence.RoutesAt),
		"报告对应固定快照，不随连接刷新。", body}, "\n")
}
