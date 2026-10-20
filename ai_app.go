package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/linuxhub/ze-ai-skywatch/internal/ai"
	"github.com/linuxhub/ze-ai-skywatch/internal/monitor"
	"github.com/linuxhub/ze-ai-skywatch/internal/ports"
	"github.com/linuxhub/ze-ai-skywatch/internal/settings"
)

type providerState struct {
	editing    bool
	draft      ai.Provider
	preset     string
	showKey    bool
	models     []string
	choice     string
	busy       bool
	status     string
	statusOK   bool
	tested     ai.Provider
	cancel     context.CancelFunc
	generation uint64
}

type analysisState struct {
	tab                        string
	pending, loading, preview  bool
	text, err, source, payload string
	cancel                     context.CancelFunc
	sequence                   uint64
}

func (a *app) activeProvider() (ai.Provider, bool) {
	for _, p := range a.config.Providers {
		if p.ID == a.config.ActiveProvider {
			return p, true
		}
	}
	return ai.Provider{}, false
}

func (a *app) editProvider(p ai.Provider) {
	if a.providerUI.cancel != nil {
		a.providerUI.cancel()
	}
	g := a.providerUI.generation + 1
	a.providerUI = providerState{editing: true, draft: p, preset: "自定义服务", generation: g}
}

func (a *app) addProvider() {
	a.editProvider(ai.Provider{ID: rand.Text(), Name: "自定义服务", Protocol: ai.OpenAI, AutoModel: true})
}

func (a *app) providerPreset() {
	p := &a.providerUI.draft
	p.APIKey = ""
	p.Protocol = ai.OpenAI
	p.Model = ""
	p.AutoModel = true
	switch a.providerUI.preset {
	case "OpenAI":
		p.Name = "OpenAI"
		p.BaseURL = "https://api.openai.com/v1"
	case "DeepSeek":
		p.Name = "DeepSeek"
		p.BaseURL = "https://api.deepseek.com/v1"
	case "Anthropic":
		p.Name = "Anthropic"
		p.BaseURL = "https://api.anthropic.com/v1"
		p.Protocol = ai.Anthropic
	default:
		p.Name = "自定义服务"
		p.BaseURL = ""
	}
	a.providerUI.models = nil
	a.providerUI.status = ""
}

// Save a candidate first, so a failed disk write never appears as a successful provider save.
func (a *app) saveAIConfig(next settings.Settings) error {
	if a.configPath != "" {
		if err := settings.Save(a.configPath, next); err != nil {
			return err
		}
	}
	a.config = next
	a.workerConfig.Store(next)
	return nil
}

func (a *app) saveProvider() {
	p := a.providerUI.draft
	p.Name, p.BaseURL, p.APIKey, p.Model = strings.TrimSpace(p.Name), strings.TrimRight(strings.TrimSpace(p.BaseURL), "/"), strings.TrimSpace(p.APIKey), strings.TrimSpace(p.Model)
	if err := p.Validate(); err != nil {
		a.providerUI.status = err.Error()
		a.providerUI.statusOK = false
		return
	}
	next := a.config
	next.Providers = append([]ai.Provider(nil), a.config.Providers...)
	found := false
	for i := range next.Providers {
		if next.Providers[i].ID == p.ID {
			next.Providers[i] = p
			found = true
			break
		}
	}
	if !found {
		next.Providers = append(next.Providers, p)
	}
	if next.ActiveProvider == "" {
		next.ActiveProvider = p.ID
	}
	if err := a.saveAIConfig(next); err != nil {
		a.providerUI.status = "保存失败：" + err.Error()
		a.providerUI.statusOK = false
		return
	}
	if a.providerUI.cancel != nil {
		a.providerUI.cancel()
	}
	a.providerUI.editing = false
	a.notify("模型提供商已保存")
}

func (a *app) removeProvider(id string) {
	next := a.config
	next.Providers = nil
	for _, p := range a.config.Providers {
		if p.ID != id {
			next.Providers = append(next.Providers, p)
		}
	}
	if next.ActiveProvider == id {
		next.ActiveProvider = ""
		if len(next.Providers) > 0 {
			next.ActiveProvider = next.Providers[0].ID
		}
	}
	if err := a.saveAIConfig(next); err != nil {
		a.notify("删除失败：" + err.Error())
		return
	}
	if a.providerUI.draft.ID == id {
		if a.providerUI.cancel != nil {
			a.providerUI.cancel()
		}
		a.providerUI.editing = false
	}
	a.notify("提供商已删除")
}

func (a *app) providerRequest(discover bool) {
	p := a.providerUI.draft
	// Discovery doesn't require an already selected model.
	validation := p
	if discover {
		validation.AutoModel = true
	}
	if err := validation.Validate(); err != nil {
		a.providerUI.status = err.Error()
		a.providerUI.statusOK = false
		return
	}
	if a.win == nil {
		return
	}
	if a.providerUI.cancel != nil {
		a.providerUI.cancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	a.providerUI.cancel = cancel
	a.providerUI.generation++
	g := a.providerUI.generation
	a.providerUI.busy = true
	a.providerUI.status = "正在测试模型…"
	if discover {
		a.providerUI.status = "正在读取模型列表…"
	}
	a.providerUI.statusOK = false
	win := a.win
	go func() {
		defer cancel()
		client := ai.NewClient()
		start := time.Now()
		var models []string
		var model string
		var err error
		if discover {
			models, err = client.Models(ctx, validation)
		} else {
			model, err = client.ResolveModel(ctx, p)
			if err == nil {
				_, err = client.Complete(ctx, p, model, "你是连接测试助手。", "请只回复 OK。")
			}
		}
		elapsed := time.Since(start)
		win.Update(func() {
			if g != a.providerUI.generation {
				return
			}
			a.providerUI.busy = false
			if a.providerUI.draft != p || !a.providerUI.editing {
				a.providerUI.status = "配置已更改，请重新测试"
				a.providerUI.statusOK = false
				return
			}
			a.providerUI.tested = p
			if err != nil {
				a.providerUI.status = err.Error()
				a.providerUI.statusOK = false
				return
			}
			a.providerUI.statusOK = true
			if discover {
				a.providerUI.models = models
				a.providerUI.choice = "选择一个模型…"
				a.providerUI.status = fmt.Sprintf("已读取 %d 个模型，可从列表选择", len(models))
			} else {
				a.providerUI.status = fmt.Sprintf("连接成功 · %s · %.1f 秒，模型已返回文本", model, elapsed.Seconds())
			}
		})
	}()
}

func (a *app) cancelAnalysis() {
	if a.analysis.cancel != nil {
		a.analysis.cancel()
		a.analysis.cancel = nil
	}
	a.analysis.sequence++
	a.analysis.loading = false
	a.analysis.pending = false
}

func (a *app) openProcessAI(p monitor.Process, port *ports.Listener) {
	a.openProcess(p, port)
	a.analysis.tab = "AI 分析"
	a.analysis.pending = a.detailLoading
	if !a.detailLoading && a.win != nil {
		a.startAnalysis()
	}
}

func (a *app) openPortAI(p ports.Listener) {
	a.openPort(p)
	a.analysis.tab = "AI 分析"
	a.analysis.pending = a.detailLoading
	if !a.detailLoading && a.win != nil {
		a.startAnalysis()
	}
}

var userPath = regexp.MustCompile(`/Users/[^/]+`)

var windowsUserPath = regexp.MustCompile(`(?i)([a-z]:[\\/](?:Users|Documents and Settings)[\\/])[^\\/]+`)

func redactUserPath(path string) string {
	path = userPath.ReplaceAllString(path, "/Users/<user>")
	return windowsUserPath.ReplaceAllString(path, "${1}<user>")
}

// This is the entire data payload: deliberately omit command lines, usernames,
// environment variables, file contents and unrelated processes.
func (a *app) analysisPayload() string {
	d, p := a.detail, a.detailProcess
	data := map[string]any{
		"os": platformName(), "name": d.Name, "pid": d.PID,
		"executable_path": redactUserPath(d.Path),
		"sampled_at":      a.detailAt, "cpu_percent_one_core": nil, "rss_bytes": nil, "threads": nil,
		"started_at": nil, "listeners": []map[string]any{}, "listeners_sampled_at": a.portSnapshot.At,
		"limitations": "一次资源采样；仅含可读取的 TCP 监听端点；未提供命令行、环境变量、签名、文件内容或全部网络连接。",
	}
	if p.CPUReady {
		data["cpu_percent_one_core"] = p.CPU
	}
	if p.MemoryReadable {
		data["rss_bytes"] = p.Memory
	}
	if d.Threads > 0 {
		data["threads"] = d.Threads
	}
	if d.Started > 0 {
		data["started_at"] = time.UnixMilli(d.Started)
	}
	listeners := []map[string]any{}
	for _, l := range a.portSnapshot.Listeners {
		if l.PID == d.PID && l.Started != 0 && l.Started == d.Started {
			listeners = append(listeners, map[string]any{"port": l.Port, "address": l.Address, "protocol": l.Protocol, "family": l.Family, "loopback_only": l.LocalOnly()})
		}
	}
	if l := a.detailPort; l != nil {
		data["selected_listener"] = map[string]any{"port": l.Port, "address": l.Address, "protocol": l.Protocol, "family": l.Family, "identity_verified": l.Started != 0 && l.Started == d.Started}
	}
	data["listeners"] = listeners
	raw, _ := json.MarshalIndent(data, "", "  ")
	return string(raw)
}

func (a *app) startAnalysis() {
	if a.detailLoading || !a.showDetail {
		return
	}
	p, ok := a.activeProvider()
	if !ok {
		a.analysis.err = "请先在设置中添加并启用一个模型提供商"
		return
	}
	if err := p.Validate(); err != nil {
		a.analysis.err = err.Error()
		return
	}
	if a.detailInspectError != "" {
		a.analysis.err = a.detailInspectError
		return
	}
	if a.win == nil {
		return
	}
	a.cancelAnalysis()
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	a.analysis.cancel = cancel
	a.analysis.loading = true
	a.analysis.text, a.analysis.err, a.analysis.source = "", "", ""
	a.analysis.payload = a.analysisPayload()
	payload := a.analysis.payload
	gen, seq := a.detailGeneration, a.analysis.sequence
	win := a.win
	go func() {
		defer cancel()
		client := ai.NewClient()
		model, err := client.ResolveModel(ctx, p)
		var result string
		if err == nil {
			result, err = client.Complete(ctx, p, model, ai.AnalysisPrompt, "请分析以下进程快照 JSON：\n"+payload)
		}
		win.Update(func() {
			if gen != a.detailGeneration || seq != a.analysis.sequence || !a.showDetail {
				return
			}
			a.analysis.loading = false
			a.analysis.cancel = nil
			if err != nil {
				a.analysis.err = err.Error()
				return
			}
			a.analysis.text = result
			a.analysis.source = p.Name + " · " + model + " · " + time.Now().Format("15:04:05")
		})
	}()
}
