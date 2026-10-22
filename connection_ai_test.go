package main

import (
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/ai"
	"github.com/linuxhub/ze-ai-skywatch/internal/appidentity"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
	"github.com/linuxhub/ze-ai-skywatch/internal/ipnotes"
)

func fixtureConnectionReport(a *app) {
	state := &a.connectionAI
	state.tab, state.prepared, state.structured = 2, true, true
	state.source = "界面示例 · 未调用模型"
	state.evidence.Identity = appidentity.Info{AppName: "Example Browser", Version: "1.2.3", BundleID: "com.example.browser", Signer: "Developer ID Application: Example Inc.", TeamID: "EXAMPLE123", Signature: "磁盘文件签名校验通过（非安全评级）", Parent: "launchd", Executable: "/Applications/Example Browser.app/Contents/MacOS/Example Browser"}
	state.evidence.IdentityAt = time.Now()
	state.report = ai.ConnectionReport{
		Summary:     "这条连接来自 Example Browser 的辅助进程，目标为局域网服务。目标命中 utun4 隧道路由，但这不等于已经验证实际流量经过 VPN。",
		Application: ai.ConnectionApplication{Name: "Example Browser（依据应用包信息）", Company: "Example Inc.（签名声明，非独立归属认证）", Role: "浏览器辅助进程；可能承担网络通信任务，具体用途仍需应用日志确认。", Basis: "结合应用包名称、Bundle ID、可执行路径与签名声明判断；名称本身不能证明身份。"},
		Connection:  "本地端点连接至局域网 IP 的 443 端口，TCP 状态为 ESTABLISHED。仅凭端口不能确定 HTTPS，也不能还原网址或请求内容。",
		Route:       "关联接口为 utun4；目标命中 192.168.110.0/24，经 10.6.0.13 转发。按系统路由推断走 VPN／隧道，无法确认具体 VPN 客户端或真实报文路径。",
		Findings:    []string{"应用信息与签名声明相互吻合，但签名校验不等于安全鉴定。", "目标命中较具体的隧道路由，而非物理网卡默认出口。"},
		Unknowns:    []string{"无法确认本地或远端哪一方主动发起连接。", "无法确定访问的域名、请求内容及是否使用应用自设代理。"},
		Suggestions: []string{"如需验证实际出口，可对目标接口进行抓包并核对时间。", "结合 VPN 客户端日志确认该隧道对应的连接配置。"},
	}
	encoded, _ := json.Marshal(state.report)
	state.text = string(encoded)
	state.payload = connectionPayload(state.evidence, true, false)
}

func TestConnectionPayloadPrivacyAndFrozenEvidence(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	fixtureSockets(a)
	a.netconfig.snapshot.Routing = fixtureRouteTable()
	row := a.connections.snapshot.Rows[0]
	row.Remote.IP, row.Interface = "192.168.110.20", "utun4"
	a.connections.detail, a.connections.detailOpen = row, true
	a.syncConnectionAnalysis()
	evidence := a.connectionAI.evidence
	evidence.Identity = appidentity.Info{Executable: "/Users/private-person/Applications/App.app/Contents/MacOS/App", AppPath: `C:\Users\private-person\App`, ProcessName: row.Process, Signature: "未知"}
	evidence.Note = ipnotes.Note{Name: "业务系统", Description: "内部地址 192.168.110.20 /Users/private-person"}
	redacted := connectionPayload(evidence, true, false)
	if !json.Valid([]byte(redacted)) {
		t.Fatal("redaction corrupted JSON", redacted)
	}
	for _, secret := range []string{"private-person", row.Local.IP, row.Remote.IP, "10.6.0.13", "192.168.110.0", "业务系统", "内部地址"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("payload leaked %q", secret)
		}
	}
	if !strings.Contains(redacted, "utun4") || !strings.Contains(redacted, "443") || !strings.Contains(redacted, "局域网") || !strings.Contains(redacted, "/24") {
		t.Fatal("privacy removed useful route facts")
	}
	annotated := connectionPayload(evidence, true, true)
	if !json.Valid([]byte(annotated)) || !strings.Contains(annotated, "业务系统") || strings.Contains(annotated, row.Remote.IP) || strings.Contains(annotated, "private-person") {
		t.Fatal("annotation opt-in did not respect redaction", annotated)
	}
	full := connectionPayload(evidence, false, false)
	if !strings.Contains(full, row.Remote.IP) || strings.Contains(full, "private-person") {
		t.Fatal("original-IP option or path redaction failed")
	}
	if redacted != connectionPayload(evidence, true, false) {
		t.Fatal("preview must be deterministic")
	}
	sequence := a.connectionAI.sequence
	if !a.connectionResultCurrent(a.connectionAI.key, sequence) {
		t.Fatal("current report rejected")
	}
	a.netconfig.snapshot.Routing.Rows = nil
	a.connections.snapshot.At = time.Now().Add(time.Minute)
	a.syncConnectionAnalysis()
	if a.connectionAI.sequence != sequence || a.connectionAI.evidence.RoutesAt != evidence.RoutesAt || a.connectionAI.evidence.Route.Rule.Interface != "utun4" || a.connectionAI.evidence.SampledAt == a.connections.snapshot.At {
		t.Fatal("background refresh mutated frozen evidence")
	}
	cancelled := false
	a.connectionAI.cancel = func() { cancelled = true }
	a.connectionAI.loading = true
	a.connections.detailOpen = false
	a.syncConnectionAnalysis()
	if !cancelled || a.connectionAI.loading || a.connectionAI.key != "" || a.connectionAI.sequence <= sequence {
		t.Fatal("closing must cancel and invalidate pending report")
	}
}

func TestConnectionReportRejectsObsoleteResults(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	fixtureSockets(a)
	a.connections.detail, a.connections.detailOpen = a.connections.snapshot.Rows[0], true
	a.syncConnectionAnalysis()
	key, sequence := a.connectionAI.key, a.connectionAI.sequence
	a.cancelConnectionAnalysis()
	if a.connectionResultCurrent(key, sequence) {
		t.Fatal("cancelled result accepted")
	}
	sequence = a.connectionAI.sequence
	a.connections.detail = a.connections.snapshot.Rows[1]
	if a.connectionResultCurrent(key, sequence) {
		t.Fatal("result for prior socket accepted before next render")
	}
	a.syncConnectionAnalysis()
	if a.connectionResultCurrent(key, sequence) {
		t.Fatal("result for prior socket accepted after next render")
	}
}

func TestConnectionPayloadIPv6AndUnavailableProcess(t *testing.T) {
	evidence := connectionEvidence{Socket: connections.Connection{Process: "unknown", Protocol: "UDP", Local: connections.Endpoint{IP: "fe80::1%en0", Port: "53"}, Remote: connections.Endpoint{IP: "2001:db8::2", Port: "53"}}, IdentityError: "权限不足"}
	payload := connectionPayload(evidence, true, false)
	if !json.Valid([]byte(payload)) || strings.Contains(payload, "fe80::1") || strings.Contains(payload, "2001:db8::2") || !strings.Contains(payload, "权限不足") {
		t.Fatal(payload)
	}
	evidence.Socket.Remote = connections.Endpoint{}
	payload = connectionPayload(evidence, true, false)
	if !strings.Contains(payload, `"remote_ip": ""`) || !strings.Contains(payload, `"state": "已绑定"`) {
		t.Fatal("listener must not invent a remote request", payload)
	}
}

func TestConnectionReportDefaultTransport(t *testing.T) {
	for _, protocol := range []string{ai.OpenAI, ai.Anthropic} {
		t.Run(protocol, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var data map[string]any
				if err := json.NewDecoder(request.Body).Decode(&data); err != nil {
					t.Error(err)
				}
				if data["model"] != "configured-model" {
					t.Error("did not use configured model")
				}
				messages := data["messages"].([]any)
				if protocol == ai.OpenAI {
					if messages[0].(map[string]any)["content"] != ai.ConnectionPrompt || messages[1].(map[string]any)["content"] != "请解释此连接快照 JSON：\n{\"fixture\":true}" {
						t.Error("prompt or payload changed")
					}
				} else if data["system"] != ai.ConnectionPrompt || len(messages) != 1 {
					t.Error("Anthropic prompt mapping failed")
				}
				writer.Header().Set("Content-Type", "application/json")
				if protocol == ai.OpenAI {
					writer.Write([]byte(`{"choices":[{"message":{"content":"分析完成"}}]}`))
				} else {
					writer.Write([]byte(`{"content":[{"type":"text","text":"分析完成"}]}`))
				}
			}))
			defer server.Close()
			provider := ai.Provider{Name: "local test", Protocol: protocol, BaseURL: server.URL, Model: "configured-model"}
			text, model, err := requestConnectionReport(context.Background(), provider, `{"fixture":true}`)
			if err != nil || text != "分析完成" || model != provider.Model {
				t.Fatal(text, model, err)
			}
		})
	}
}

func TestConnectionReportLayoutsAndNavigation(t *testing.T) {
	for _, size := range [][2]int{{1180, 700}, {1360, 850}} {
		for _, dark := range []bool{false, true} {
			a := fixtureApp()
			a.page = "connections"
			fixtureSockets(a)
			a.netconfig.snapshot.Routing = fixtureRouteTable()
			a.connections.detail, a.connections.detailOpen = a.connections.snapshot.Rows[0], true
			tester := ui.NewTester(a.view, size[0], size[1])
			tester.SetLocale("zh-Hans")
			tester.SetDark(dark)
			// The table also contains AI 分析 labels. Focus the adjacent
			// modal tab and use its keyboard navigation to target this tab.
			if err := tester.Click("路由分析"); err != nil {
				t.Fatal(err)
			}
			tester.Key(0, ui.KeyRight)
			if !tester.HasText("智能连接报告") || !tester.HasText("本机证据") || a.connectionAI.loading {
				t.Fatal("report must start without model request")
			}
			for _, phase := range []string{"empty", "report"} {
				if phase == "report" {
					fixtureConnectionReport(a)
				}
				tester.Frame()
				for _, label := range []string{"关闭端点详情", "复制报告", "结束此进程…"} {
					bounds, ok := tester.Find(label)
					if !ok || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > float32(size[0]) || bounds.Y+bounds.H > float32(size[1]) {
						t.Fatalf("%s clipped at %v: %+v", label, size, bounds)
					}
				}
				if directory := os.Getenv("MAC_MONITOR_SCREENSHOTS"); directory != "" {
					if err := os.MkdirAll(directory, 0755); err != nil {
						t.Fatal(err)
					}
					name := "connection-ai-" + phase + map[bool]string{false: "-light", true: "-dark"}[dark]
					if size[0] == 1180 {
						name += "-compact"
					}
					file, err := os.Create(filepath.Join(directory, name+".png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tester.Image())
					file.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tester.Click("复制报告"); err != nil || !strings.Contains(tester.Clipboard(), "界面示例 · 未调用模型") {
				t.Fatal("copy report failed", err)
			}
			if !strings.Contains(tester.Clipboard(), "应用身份\n应用：") || strings.Contains(tester.Clipboard(), `"summary":`) {
				t.Fatal("structured report must copy as readable text, not JSON")
			}
			body, ok := tester.Find("连接详情内容")
			if !ok {
				t.Fatal("missing report scroll area")
			}
			tester.Scroll(body.X+body.W/3, body.Y+body.H/2, 0, 10000)
			tester.Frame()
			last, ok := tester.Find("结合 VPN 客户端日志确认该隧道对应的连接配置。")
			if !ok || last.Y < body.Y || last.Y+last.H > body.Y+body.H+1 {
				t.Fatalf("last report card cannot be reached at %v: %+v within %+v", size, last, body)
			}
			for _, tab := range []string{"连接信息", "路由分析", "AI 分析"} {
				if tab == "AI 分析" {
					tester.Key(0, ui.KeyRight)
					continue
				}
				if err := tester.Click(tab); err != nil {
					t.Fatal(err)
				}
			}
			tester.Key(0, ui.KeyEscape)
			if a.connections.detailOpen || a.connectionAI.key != "" {
				t.Fatal("Escape did not dismiss and clear report")
			}
			a.cancel()
		}
	}
}
