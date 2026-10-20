package main

import "github.com/egoist/mygo/ui"

func (a *app) aboutPage(c *ui.Context) {
	ui.Text(c, "关于").FontSize(25).Bold().Shrink(0)
	services := c.Services()
	linkColor := blue
	if c.Theme().Dark {
		linkColor = ui.Hex("#78b7ff")
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Gap(18).Children(func() {
		surface, _ := a.palette(c)
		ui.Column(c).Padding(28).Gap(24).Radius(11).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Children(func() {
			ui.Row(c).Gap(20).AlignItems(ui.Center).Children(func() {
				ui.Image(c, appIcon).Size(80, 80).Fit(ui.Contain).Shrink(0)
				ui.Column(c).Grow(1).Gap(7).Children(func() {
					ui.Text(c, appName).FontSize(26).Bold()
					ui.Text(c, appEnglishName).FontSize(14).TextColor(c.Theme().TextMuted)
					ui.Text(c, appTagline).FontSize(14).TextColor(c.Theme().TextMuted)
				})
				ui.Text(c, "版本 "+appVersion).FontSize(12).TextColor(linkColor).Padding(6, 10).Radius(6).Background(blue.Alpha(0.08))
			})
		})
		a.settingsCard(c, "作者与项目", func() {
			a.detailRow(c, "作者", appAuthor)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "代码仓库").Width(90).Shrink(0).FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Link(c.Key("repository-link"), appRepositoryURL, appRepositoryURL).FontSize(12).TextColor(linkColor)
				ui.Spacer(c)
				ui.Button(c, "复制地址").Label("复制仓库地址").OnClick(func() {
					services.WriteClipboard(appRepositoryURL)
					a.notify("已复制仓库地址")
				})
			})
		})
		ui.Text(c, "使用 Go 与 MyGo 原生界面构建。").FontSize(11).TextColor(c.Theme().TextMuted)
	})
}
