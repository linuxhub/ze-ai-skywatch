package main

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/ai"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/linuxhub/ze-ai-skywatch/internal/ports"
)

func TestAIEntryAndProviderEditing(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	tt := ui.NewTester(a.view, 1180, 700)
	tt.SetLocale("zh-Hans")
	if err := tt.Click("分析进程 Google Chrome PID 201"); err != nil {
		t.Fatal(err)
	}
	if !a.showDetail || a.detail.PID != 201 || a.analysis.tab != "AI 分析" || !tt.HasText("AI 分析结果") {
		t.Fatal("row AI opened wrong process")
	}
	if err := tt.Click("模型设置"); err != nil {
		t.Fatal(err)
	}
	if a.page != "settings" || a.showDetail {
		t.Fatal("settings routing failed")
	}
	if err := tt.Click("添加提供商"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("配置模型提供商") {
		t.Fatal("provider editor missing")
	}
	p := ai.Provider{ID: a.providerUI.draft.ID, Name: "测试提供商", Protocol: ai.OpenAI, BaseURL: "https://example.com/v1", APIKey: "test-key", Model: "gpt-5.6"}
	for _, field := range []struct{ label, value string }{{"提供商名称", p.Name}, {"API 端点", p.BaseURL}, {"API 密钥", p.APIKey}, {"模型 ID", p.Model}} {
		if err := tt.Click("输入 " + field.label); err != nil {
			t.Fatal(err)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(field.value)
	}
	if err := tt.Click("自动从服务列表选择文本模型"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("保存提供商"); err != nil {
		t.Fatal(err)
	}
	if a.providerUI.editing || len(a.config.Providers) != 1 || a.config.ActiveProvider != p.ID || a.config.Providers[0] != p {
		t.Fatal("provider not saved or activated with typed values")
	}
	// Exercise extra builds as the modal disappears and returns.
	tt.Frame()
	a.editProvider(p)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.removeProvider(p.ID)
	tt.Frame()
	tt.Frame()
	if len(a.config.Providers) != 0 || a.config.ActiveProvider != "" {
		t.Fatal("default not cleared after deleting last provider")
	}
	port := a.portSnapshot.Listeners[3]
	a.openPortAI(port)
	tt.Frame()
	tt.Frame()
	if a.detailPort == nil || a.detailPort.Port != 3306 || a.analysis.tab != "AI 分析" {
		t.Fatal("port AI opened wrong listener")
	}
}

func TestAnalysisPayloadMinimizesDataAndRejectsReusedPID(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.detailProcess = monitor.Process{Identity: monitor.Identity{PID: 123, Started: 1000}, Name: "demo"}
	a.detail = monitor.Detail{Identity: a.detailProcess.Identity, Name: "demo", Path: "/Users/private-name/app/bin", Command: "--api-key super-secret", User: "private-name"}
	a.portSnapshot.Listeners = []ports.Listener{
		{PID: 123, Started: 1000, Port: 8080, Address: "127.0.0.1", Protocol: "TCP"},
		{PID: 123, Started: 2000, Port: 9999, Address: "*", Protocol: "TCP"},
	}
	raw := a.analysisPayload()
	for _, secret := range []string{"super-secret", "private-name", "9999"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("payload leaked %s", secret)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["cpu_percent_one_core"] != nil || payload["rss_bytes"] != nil || len(payload["listeners"].([]any)) != 1 {
		t.Fatalf("incorrect missing data / process identity: %s", raw)
	}
	if !strings.Contains(raw, "<user>") && !strings.Contains(raw, `\u003cuser\u003e`) {
		t.Fatal("username not sanitized")
	}
}

func TestRenderAI(t *testing.T) {
	dir := os.Getenv("MAC_MONITOR_SCREENSHOTS")
	for _, dark := range []bool{false, true} {
		for _, size := range [][2]int{{1180, 700}, {1360, 850}} {
			a := fixtureApp()
			p := ai.Provider{ID: "fixture", Name: "示例模型服务", Protocol: ai.OpenAI, BaseURL: "https://example.com/v1", Model: "gpt-5.6"}
			a.config.Providers = []ai.Provider{p}
			a.config.ActiveProvider = p.ID
			tt := ui.NewTester(a.view, size[0], size[1])
			tt.SetLocale("zh-Hans")
			tt.SetDark(dark)
			for _, screen := range []string{"ai-settings", "ai-provider", "ai-result", "ai-preview"} {
				a.showDetail = false
				a.providerUI.editing = false
				if screen == "ai-settings" {
					a.page = "settings"
				} else if screen == "ai-provider" {
					a.editProvider(p)
				} else {
					a.openProcessAI(a.snapshot.Processes[1], nil)
					a.analysis.source = "示例模型服务 · gpt-5.6 · 测试预览"
					a.analysis.text = "## 这是什么进程\nGoogle Chrome 是网页浏览器进程，通常负责网页渲染和扩展运行。\n## 资源使用分析\n当前 CPU 使用率 29%，RSS 492 MB。单次采样无法判断是否持续异常。\n## 端口与风险\n当前摘要未提供匹配的监听端点，不能据此认定没有联网。\n## 建议\n- 在浏览器任务管理器中检查标签页与扩展。\n- 连续观察资源占用，再决定是否关闭相关标签页。"
					a.analysis.preview = screen == "ai-preview"
				}
				tt.Frame()
				tt.Frame()
				if tt.FramePending() {
					t.Errorf("%s did not settle", screen)
				}
				if dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					name := screen + map[bool]string{false: "-light", true: "-dark"}[dark]
					if size[0] == 1180 {
						name += "-compact"
					}
					f, err := os.Create(filepath.Join(dir, name+".png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, tt.Image())
					f.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a.cancel()
		}
	}
}
