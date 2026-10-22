package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/connections"
)

func (a *app) socketRouteView(c *ui.Context, row connections.Connection) {
	match := a.matchRoute(row.Remote.IP)
	a.reportCard(c, "当前路由证据", "系统规则 · 非抓包结果", func() {
		a.detailRow(c, "网络接口", placeholder(row.Interface)+"（采样关联或本地绑定）")
		a.detailRow(c, "路由出口推断", match.Summary())
		if match.Found {
			a.detailRow(c, "匹配网段", match.Rule.Destination)
			a.detailRow(c, "路由下一跳", match.Rule.NextHop())
		}
		a.detailRow(c, "路由采集时间", sinceLabel(a.netconfig.snapshot.Routing.At))
		a.detailRow(c, "系统代理配置", a.netconfig.snapshot.ProxyStatus())
	})
	text := "当前没有充分证据确认此连接使用 VPN。普通接口出口也不能排除应用自设代理。"
	if match.Found && match.Rule.Tunnel && !match.Rule.Reject {
		text = "目标命中 VPN／隧道路由，系统出口推断为 " + match.Rule.Interface + "。这不等于已验证报文经过 VPN，也不能仅凭接口名确认 Tunnelblick 或 OpenVPN。"
	}
	ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted).Padding(15).Radius(9).Background(purple.Alpha(0.06))
	ui.Text(c, "应用绑定接口、既有连接与自设代理可能使用不同路径。AI 分析使用打开详情时冻结的路由快照，与此页当前刷新结果可能不同。").FontSize(11).TextColor(c.Theme().TextMuted)
}

func (a *app) reportCard(c *ui.Context, title, caption string, content func()) {
	surface, _ := a.palette(c)
	ui.Column(c).Padding(16).Gap(10).Radius(10).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Selectable().Children(func() {
		ui.Text(c, title).FontSize(14).Bold()
		if caption != "" {
			ui.Text(c, caption).FontSize(10).TextColor(c.Theme().TextMuted)
		}
		content()
	})
}

func reportField(c *ui.Context, label, value string) {
	ui.Column(c.Key(label)).Gap(4).Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted)
		ui.Text(c, placeholder(value)).FontSize(12)
	})
}

func reportList(c *ui.Context, items []string) {
	if len(items) == 0 {
		ui.Text(c, "模型未提供此项").FontSize(11).TextColor(c.Theme().TextMuted)
	}
	for index, item := range items {
		ui.Row(c.Key(index)).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Text(c, "·").TextColor(purple).Width(8).Shrink(0)
			ui.Text(c, item).FontSize(12).Grow(1).MinWidth(0)
		})
	}
}

func (a *app) connectionAnalysisView(c *ui.Context) {
	state := &a.connectionAI
	evidence := state.evidence
	provider, configured := a.activeProvider()
	services := c.Services()
	accent := purple
	if c.Theme().Dark {
		accent = ui.Hex("#c1adff")
	}
	ui.Row(c).Gap(12).Children(func() {
		ui.Icon(c, aiIcon).Size(25, 25).TextColor(accent)
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, "智能连接报告").FontSize(18).Bold()
			label := "使用本程序的默认模型，解读应用身份与网络路径"
			if configured {
				label = "默认服务：" + provider.Name + " · " + provider.Model
				if provider.AutoModel {
					label = "默认服务：" + provider.Name + " · 自动选择模型"
				}
			}
			if state.source != "" {
				label = state.source
			}
			ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted)
		})
		if state.loading {
			ui.Button(c, "取消分析").OnClick(func() { a.cancelConnectionAnalysis(); state.err = "已取消；没有生成报告" })
		} else {
			label := "开始 AI 分析"
			if state.text != "" {
				label = "重新分析"
			}
			ui.PrimaryButton(c, label).Disabled(state.preparing).OnClick(a.startConnectionAnalysis)
		}
		ui.Button(c, "复制报告").Disabled(state.text == "").OnClick(func() { services.WriteClipboard(a.connectionReportText()); a.notify("已复制 AI 连接报告") })
	})
	ui.Column(c).Padding(12, 14).Gap(8).Radius(8).Background(accent.Alpha(0.05)).Children(func() {
		ui.Row(c).Gap(16).Children(func() {
			ui.Checkbox(c.Key("socket-redact-ip"), &state.redactIPs, "IP 地址脱敏").Disabled(state.loading || state.text != "").OnChange(func() { state.payload = "" })
			ui.Checkbox(c.Key("socket-include-note"), &state.includeNote, "附带自定义 IP 标注").Disabled(state.loading || state.text != "").OnChange(func() { state.payload = "" })
			ui.Spacer(c)
			if state.text != "" {
				ui.Button(c, "调整发送选项").OnClick(func() {
					state.text, state.source, state.payload, state.structured = "", "", "", false
				})
			}
			ui.Checkbox(c.Key("socket-payload-preview"), &state.preview, "查看发送的数据")
		})
		ui.Text(c, "仅主动分析时发送给默认模型；应用名、签名与去用户名的路径仍会发送。无请求内容、命令行或环境变量。报告生成后保留原发送选项。").FontSize(10).TextColor(c.Theme().TextMuted)
	})
	if state.preview {
		payload := state.payload
		if payload == "" {
			payload = connectionPayload(evidence, state.redactIPs, state.includeNote)
		}
		a.reportCard(c, "发送数据预览", "原样发送下方 JSON；不包含 API 密钥", func() {
			ui.Text(c, payload).FontSize(10)
		})
	}
	if !configured && state.text == "" {
		ui.Row(c).Gap(12).Padding(12).Radius(8).Background(c.Theme().Warning.Alpha(0.08)).Children(func() {
			ui.Text(c, "尚未配置默认模型服务").FontSize(12).Grow(1)
			ui.Button(c, "配置分析模型").OnClick(func() { a.connections.detailOpen = false; a.page = "settings" })
		})
	}
	if state.err != "" {
		ui.Text(c, state.err).FontSize(12).TextColor(c.Theme().Warning).Padding(12).Radius(8).Background(c.Theme().Warning.Alpha(0.06))
	}
	if state.preparing || state.loading {
		label := "正在读取本机应用与签名证据…（尚未发送模型请求）"
		if state.loading {
			label = "AI 正在解读固定快照… 你可以取消，关闭详情也会取消请求。"
		}
		ui.Text(c, label).FontSize(12).TextColor(accent).Padding(16).Radius(10).Background(accent.Alpha(0.06))
	}
	if state.text != "" {
		summary := "模型返回了非结构化内容，以下按原文展示。"
		if state.structured {
			summary = state.report.Summary
		}
		ui.Column(c).Padding(20).Gap(10).Radius(12).Background(accent.Alpha(0.08)).Border(1, accent.Alpha(0.18)).Selectable().Children(func() {
			ui.Text(c, "AI INSIGHT  /  连接解读").FontSize(10).Bold().TextColor(accent)
			ui.Text(c, summary).FontSize(15).Bold()
			ui.Text(c, "模型生成 · 不代替抓包或安全鉴定 · 固定快照").FontSize(10).TextColor(c.Theme().TextMuted)
		})
	}
	ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(12).Children(func() {
			if state.text == "" {
				a.reportCard(c, "把一条连接，看成一段清楚的解释", "不是堆砌技术字段，而是回答你真正关心的问题", func() {
					for _, item := range []struct{ title, text string }{
						{"01  这是什么应用", "应用名称、可能的公司、主程序或辅助进程，并说明判断依据。"},
						{"02  从哪里连接到哪里", "解释本地与远端端点、协议、状态，不编造请求网址和内容。"},
						{"03  走哪条网络路径", "结合关联接口、网段与下一跳，说明 VPN／代理证据与未知项。"},
					} {
						ui.Text(c, item.title).FontSize(13).Bold().TextColor(accent)
						ui.Text(c, item.text).FontSize(12).TextColor(c.Theme().TextMuted)
					}
				})
			} else if !state.structured {
				a.reportCard(c, "模型原文", "未识别为结构化报告；文字不会生成可执行操作", func() { analysisText(c, state.text) })
			} else {
				report := state.report
				a.reportCard(c, "应用身份", "AI 解释 · 声明不等于已验证归属", func() {
					reportField(c, "应用", report.Application.Name)
					reportField(c, "公司／发布者", report.Application.Company)
					reportField(c, "进程角色与用途", report.Application.Role)
					reportField(c, "判断依据", report.Application.Basis)
				})
				a.reportCard(c, "连接去向", "AI 解释 · 不能据此确定谁主动发起", func() { ui.Text(c, report.Connection).FontSize(12) })
				a.reportCard(c, "网络路径与 VPN", "AI 解释 · 按路由规则推断", func() { ui.Text(c, report.Route).FontSize(12) })
				a.reportCard(c, "关键发现", "AI 整理 · 对照右侧本机证据", func() { reportList(c, report.Findings) })
				a.reportCard(c, "仍无法确认", "保留边界，不把推测写成事实", func() { reportList(c, report.Unknowns) })
				a.reportCard(c, "可选下一步", "仅建议，不会自动修改配置或执行操作", func() { reportList(c, report.Suggestions) })
			}
		})
		ui.Column(c).Width(285).Shrink(0).Gap(12).Children(func() {
			a.reportCard(c, "本机证据", "本地读取 · 不由 AI 生成", func() {
				identity := evidence.Identity
				reportField(c, "进程", fmt.Sprintf("%s · PID %d", evidence.Socket.Process, evidence.Socket.PID))
				reportField(c, "应用名／版本（软件声明）", placeholder(identity.AppName)+" · "+placeholder(identity.Version))
				if identity.DeclaredCompany != "" {
					reportField(c, "公司（软件声明）", identity.DeclaredCompany)
				}
				if identity.BundleID != "" {
					reportField(c, "Bundle ID（软件声明）", identity.BundleID)
				}
				reportField(c, "发布者（签名声明）", identity.Signer)
				if identity.TeamID != "" {
					reportField(c, "签名 Team ID", identity.TeamID)
				}
				reportField(c, "磁盘文件签名", identity.Signature)
				if identity.Parent != "" {
					reportField(c, "父进程（不等于所属应用）", identity.Parent)
				}
				if identity.Executable != "" {
					reportField(c, "可执行路径（去用户名）", redactUserPath(identity.Executable))
				}
				if identity.Warning != "" {
					ui.Text(c, identity.Warning).FontSize(11).TextColor(c.Theme().Warning)
				}
				if evidence.IdentityError != "" {
					ui.Text(c, evidence.IdentityError).FontSize(11).TextColor(c.Theme().Warning)
				}
			})
			a.reportCard(c, "快照时间线", "打开详情时冻结 · 不随刷新变化", func() {
				reportField(c, "连接采集", sinceLabel(evidence.SampledAt))
				reportField(c, "路由采集", sinceLabel(evidence.RoutesAt))
				reportField(c, "身份读取", sinceLabel(evidence.IdentityAt))
				reportField(c, "路由推断", evidence.Route.Summary())
				reportField(c, "关联接口", evidence.Socket.Interface)
				reportField(c, "系统代理配置（非使用证明）", evidence.Proxy)
				for _, warning := range evidence.CollectionWarnings {
					ui.Text(c, warning).FontSize(11).TextColor(c.Theme().Warning)
				}
			})
		})
	})
}
