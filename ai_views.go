package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/ai"
)

func (a *app) aiButton(c *ui.Context, label string, action func()) {
	b := ui.ButtonBase(c).Size(28, 26).Radius(6).Center().Background(purple.Alpha(0.09)).Label(label).Tooltip("AI " + label)
	if b.Hovered() {
		b.Background(purple.Alpha(0.18))
	}
	b.Children(func() { ui.Icon(c, aiIcon).Size(17, 17).TextColor(purple) })
	b.OnClick(action)
}

func (a *app) aiSettingsView(c *ui.Context) {
	a.settingsCard(c, "AI 模型服务", func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Icon(c, aiIcon).Size(24, 24).TextColor(purple)
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, "让 AI 帮你认识进程与端口").Bold()
				ui.Text(c, "添加模型服务，选择一个默认提供商用于分析。").FontSize(12).TextColor(c.Theme().TextMuted)
			})
			ui.PrimaryButton(c, "添加提供商").OnClick(a.addProvider)
		})
		if len(a.config.Providers) == 0 {
			ui.Column(c).Padding(20).Gap(6).Radius(8).Background(purple.Alpha(0.04)).Children(func() {
				ui.Text(c, "尚未配置模型服务").Bold()
				ui.Text(c, "支持 OpenAI、DeepSeek、Anthropic，以及使用 OpenAI 兼容接口的自定义服务。").FontSize(12).TextColor(c.Theme().TextMuted)
			})
		}
		for _, p := range a.config.Providers {
			ui.Row(c.Key(p.ID)).Padding(12).Gap(12).Radius(8).Border(1, c.Theme().Border).Children(func() {
				ui.Icon(c, aiIcon).Size(20, 20).TextColor(purple)
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					ui.Row(c).Gap(10).Children(func() {
						ui.Text(c, p.Name).Bold()
						if p.ID == a.config.ActiveProvider {
							ui.Text(c, "默认分析服务").FontSize(10).TextColor(green).Padding(3, 7).Radius(4).Background(green.Alpha(0.08))
						}
					})
					model := p.Model
					if p.AutoModel {
						model = "自动选择"
						if p.Model != "" {
							model += " · 优先 " + p.Model
						}
					}
					ui.Text(c, p.Protocol+" · "+model+" · "+p.BaseURL).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine().Tooltip(p.BaseURL)
				})
				ui.Button(c, "设为默认").Disabled(p.ID == a.config.ActiveProvider).OnClick(func() {
					next := a.config
					next.ActiveProvider = p.ID
					if err := a.saveAIConfig(next); err != nil {
						a.notify("保存失败：" + err.Error())
					}
				})
				ui.Button(c, "配置与测试").OnClick(func() { a.editProvider(p) })
				ui.Button(c, "删除").TextColor(c.Theme().Danger).OnClick(func() { a.removeProvider(p.ID) })
			})
		}
		ui.Text(c, "仅在主动点击 AI 分析时发送选定的进程或连接摘要。连接分析还可附带应用和签名证据，并预览发送内容；不会发送命令行、环境变量或文件内容。").FontSize(11).TextColor(c.Theme().TextMuted)
	})
}

func (a *app) providerEditor(c *ui.Context) {
	u := &a.providerUI
	ui.Modal(c, &u.editing, func() {
		ui.Column(c).Width(650).MaxHeight(620).Gap(14).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				ui.Icon(c, aiIcon).Size(22, 22).TextColor(purple)
				ui.Text(c, "配置模型提供商").FontSize(20).Bold().Grow(1)
				ui.Button(c, "取消").OnClick(func() {
					if u.cancel != nil {
						u.cancel()
					}
					u.editing = false
				})
			})
			ui.Scroll(c).MaxHeight(470).Gap(12).Children(func() {
				ui.Row(c).Gap(14).Children(func() {
					ui.Text(c, "快速填入").Width(100)
					ui.Select(c.Key("provider-preset"), &u.preset, []string{"自定义服务", "OpenAI", "DeepSeek", "Anthropic"}).Grow(1).Label("提供商预设").OnChange(a.providerPreset)
				})
				providerField(c, "提供商名称", &u.draft.Name, "例如：我的模型服务")
				ui.Row(c).Gap(14).Children(func() {
					ui.Text(c, "API 协议").Width(100)
					ui.Select(c.Key("provider-protocol"), &u.draft.Protocol, []string{ai.OpenAI, ai.Anthropic}).Grow(1).Label("API 协议")
				})
				providerField(c, "API 端点", &u.draft.BaseURL, "https://api.example.com/v1")
				ui.Text(c, "填写 Base URL（通常包含 /v1）；本机服务可用 http://localhost:端口/v1。").FontSize(11).TextColor(c.Theme().TextMuted)
				ui.Row(c).Gap(14).Children(func() {
					ui.Text(c, "API 密钥").Width(100)
					field := ui.TextInput(c.Key("provider-key"), &u.draft.APIKey).Grow(1).Label("输入 API 密钥").Placeholder("输入 API 密钥；无鉴权的本机服务可留空")
					if !u.showKey {
						field.Password()
					}
					ui.Checkbox(c, &u.showKey, "显示")
				})
				ui.Divider(c)
				providerField(c, "模型 ID", &u.draft.Model, "例如：gpt-5.6；使用服务提供的准确 ID")
				ui.Row(c).Gap(12).Children(func() {
					ui.Button(c, "读取模型列表").Disabled(u.busy).OnClick(func() { a.providerRequest(true) })
					if len(u.models) > 0 {
						ui.Select(c.Key("discovered-model"), &u.choice, u.models).Grow(1).Label("服务可用模型").OnChange(func() { u.draft.Model = u.choice })
					} else {
						ui.Text(c, "也可以直接手动输入模型 ID").FontSize(11).TextColor(c.Theme().TextMuted)
					}
				})
				ui.Checkbox(c, &u.draft.AutoModel, "自动从服务列表选择文本模型")
				ui.Text(c, "自动模式优先使用上面填写的模型；列表不可用时会尝试该模型。选择规则不代表质量排名。").FontSize(11).TextColor(c.Theme().TextMuted)
				if u.status != "" {
					text, color := u.status, c.Theme().Warning
					if u.busy {
						color = c.Theme().TextMuted
					} else if u.statusOK {
						if u.tested == u.draft {
							color = green
						} else {
							text = "配置已更改，请重新测试"
							color = c.Theme().TextMuted
						}
					}
					ui.Text(c, text).FontSize(12).TextColor(color).Padding(10).Radius(6).Background(color.Alpha(0.06))
				}
				ui.Text(c, "密钥保存在本机 settings.json（仅当前用户可读写，未加密）。测试会发送一条简短对话，可能产生少量用量。").FontSize(11).TextColor(c.Theme().TextMuted)
			})
			ui.Divider(c)
			ui.Row(c).Gap(10).Children(func() {
				label := "测试连接"
				if u.busy {
					label = "请求中…"
				}
				ui.Button(c, label).Disabled(u.busy).OnClick(func() { a.providerRequest(false) })
				if u.busy {
					ui.Button(c, "取消请求").OnClick(func() {
						if u.cancel != nil {
							u.cancel()
						}
						u.generation++
						u.busy = false
						u.status = "已取消请求"
						u.statusOK = false
					})
				}
				ui.Spacer(c)
				ui.PrimaryButton(c, "保存提供商").Disabled(u.busy).OnClick(a.saveProvider)
			})
		})
	})
}

func providerField(c *ui.Context, label string, value *string, hint string) {
	ui.Row(c).Gap(14).Children(func() {
		ui.Text(c, label).Width(100).Shrink(0)
		ui.TextInput(c.Key(label), value).Grow(1).MinWidth(0).Label("输入 " + label).Placeholder(hint)
	})
}

func (a *app) analysisView(c *ui.Context) {
	t := c.Theme()
	p, configured := a.activeProvider()
	ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(12).Children(func() {
			ui.Row(c).Gap(9).Children(func() {
				ui.Icon(c, aiIcon).Size(24, 24).TextColor(purple)
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					ui.Text(c, "AI 分析结果").Bold().FontSize(16)
					label := "请配置模型服务后开始分析"
					if configured {
						label = "使用 " + p.Name + " 分析这次进程采样"
					}
					if a.analysis.source != "" {
						label = a.analysis.source
					}
					ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
				})
			})
			if a.detailLoading || a.analysis.pending {
				ui.Text(c, "正在读取进程信息…").TextColor(purple)
			} else if a.analysis.loading {
				ui.Text(c, "正在请求模型，分析进程用途、资源占用和端口风险…").TextColor(purple)
			} else if a.analysis.text != "" {
				analysisText(c, a.analysis.text)
			} else if a.analysis.err == "" {
				ui.Text(c, "点击开始分析，AI 将解释进程用途、资源占用与可能的风险，并给出排查建议。").FontSize(12).TextColor(t.TextMuted)
			}
			if a.analysis.err != "" {
				ui.Text(c, a.analysis.err).TextColor(t.Warning).Padding(12).Radius(7).Background(t.Warning.Alpha(0.06))
			}
			ui.Row(c).Gap(8).Children(func() {
				if configured {
					label := "开始分析"
					if a.analysis.text != "" || a.analysis.err != "" {
						label = "重新分析"
					}
					ui.PrimaryButton(c, label).Disabled(a.analysis.loading || a.detailLoading || a.detailInspectError != "").OnClick(a.startAnalysis)
				}
				if a.analysis.loading {
					ui.Button(c, "取消分析").OnClick(func() { a.cancelAnalysis(); a.analysis.err = "分析已取消，可重新发起" })
				}
				if a.analysis.text != "" {
					ui.Button(c, "复制分析").OnClick(func() { a.copy(a.analysis.text) })
				}
				ui.Button(c, "模型设置").OnClick(func() { a.cancelAnalysis(); a.showDetail = false; a.page = "settings" })
			})
			ui.Text(c, "AI 根据有限快照推断，不能代替安全检测。分析结果不会自动执行任何操作。").FontSize(10).TextColor(t.TextMuted)
			ui.Checkbox(c, &a.analysis.preview, "查看发送的数据")
			if a.analysis.preview {
				payload := a.analysis.payload
				if payload == "" {
					payload = a.analysisPayload()
				}
				ui.Text(c, payload).FontSize(11).Padding(12).Radius(7).Background(t.Surface)
			}
		})
		ui.Column(c).Width(240).Shrink(0).Padding(15).Gap(12).Radius(9).Background(purple.Alpha(0.04)).Children(func() {
			ui.Text(c, "进程基本信息").Bold()
			a.detailRow(c, "名称", a.detail.Name)
			a.detailRow(c, "PID", fmt.Sprint(a.detail.PID))
			a.detailRow(c, "CPU", cpuText(a.detailProcess))
			a.detailRow(c, "内存 RSS", memoryText(a.detailProcess))
			if a.detail.Threads > 0 {
				a.detailRow(c, "线程", fmt.Sprint(a.detail.Threads))
			}
			if l := a.detailPort; l != nil {
				a.detailRow(c, "端口", fmt.Sprint(l.Port))
				a.detailRow(c, "监听地址", l.Address)
			}
			ui.Divider(c)
			ui.Text(c, "采样时间 "+a.detailAt.Format("15:04:05")).FontSize(10).TextColor(t.TextMuted)
			ui.Text(c, "不发送命令行、环境变量和文件内容；路径中的用户名已隐藏。").FontSize(10).TextColor(t.TextMuted)
		})
	})
}

// Render a small, inert subset of Markdown using native text widgets. Model
// output cannot create executable actions, external images or clickable links.
func analysisText(c *ui.Context, text string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		heading := strings.HasPrefix(line, "#")
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		line = strings.ReplaceAll(line, "**", "")
		if strings.HasPrefix(line, "- ") {
			line = "• " + strings.TrimPrefix(line, "- ")
		}
		e := ui.Text(c, line).FontSize(12)
		if heading {
			e.Bold().FontSize(14).Margin(7, 0, 0)
		}
	}
}
