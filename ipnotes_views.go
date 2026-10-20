package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/ipnotes"
)

func (a *app) ipNoteBadge(c *ui.Context, note ipnotes.Note) {
	ui.Text(c, "◇ "+note.Name).FontSize(10).TextColor(purple).SingleLine().Padding(3, 6).Radius(4).Background(purple.Alpha(0.09)).Tooltip(note.IP + " · " + note.Name + "\n" + note.Group + "\n" + note.Description)
}
func (a *app) ipNotesSettings(c *ui.Context) {
	surface, _ := a.palette(c)
	ui.Column(c.Key("ip-notes-panel")).Grow(1).Basis(0).MinHeight(0).Padding(20).Gap(15).Radius(11).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Children(func() {
		ui.Text(c, "IP 标注簿").Bold().FontSize(14).Shrink(0)
		ui.Divider(c).Shrink(0)
		ui.Row(c).Shrink(0).Gap(12).Children(func() {
			ui.Column(c).Grow(1).Gap(5).Children(func() {
				ui.Text(c, "为连接地址添加业务说明").Bold()
				ui.Text(c, "标注保存在本机，支持 IPv4 / IPv6 精确匹配，与联网位置查询独立。").FontSize(11).TextColor(c.Theme().TextMuted)
			})
			ui.PrimaryButton(c, "添加 IP 标注").OnClick(func() { a.editIPNote("") })
		})
		ui.SearchField(c.Key("ip-notes-search"), &a.ipNotes.query).Placeholder("搜索 IP、名称、分组或说明…").Label("搜索 IP 标注").Shrink(0)
		rows := a.filteredIPNotes()
		a.ipNotes.list.Key = func(i int) any { return rows[i].IP }
		a.ipNotes.list.Label = func(i int) string { return rows[i].IP + " " + rows[i].Name }
		table := ui.Table(c.Key("ip-notes-table"), &a.ipNotes.list, []ui.TableColumn{
			{Title: "IP 地址", Width: 180}, {Title: "标注名称", MinWidth: 160}, {Title: "分组", Width: 160}, {Title: "操作", Width: 70},
		}, len(rows), func(row, col int) {
			n := rows[row]
			switch col {
			case 0:
				ui.Text(c, n.IP).FontSize(11).SingleLine().Tooltip(n.IP)
			case 1:
				ui.Text(c, n.Name).FontSize(11).SingleLine().Tooltip(n.Name + "\n" + n.Description)
			case 2:
				ui.Text(c, n.Group).FontSize(11).SingleLine().Tooltip(n.Group)
			case 3:
				ui.Button(c, "编辑").FontSize(10).OnClick(func() { a.editIPNote(n.IP) })
			}
		}).Grow(1).MinHeight(0).Label("IP 标注列表")
		if len(rows) == 0 {
			table.Children(func() {
				ui.Text(c, "暂无标注或没有匹配项。也可从连接行直接标注远端 IP。").Padding(16).FontSize(11).TextColor(c.Theme().TextMuted)
			})
		}
		ui.Textf(c, "共 %d 条 · 当前筛选 %d 条", len(a.config.IPNotes), len(rows)).FontSize(11).TextColor(c.Theme().TextMuted).Shrink(0)
	})
}
func (a *app) ipNoteEditor(c *ui.Context) {
	n := &a.ipNotes
	ui.Modal(c.Key("ip-note-editor"), &n.editing, func() {
		ui.Column(c).Width(620).MaxHeight(610).Gap(14).Children(func() {
			ui.Row(c).Gap(12).Children(func() {
				ui.Text(c, "编辑 IP 标注").FontSize(20).Bold().Grow(1)
				ui.Button(c, "取消标注编辑").OnClick(func() { n.editing = false })
			})
			ui.Scroll(c).MaxHeight(400).Gap(14).Children(func() {
				providerField(c, "IP 地址", &n.draft.IP, "例如：121.43.212.142")
				providerField(c, "标注名称", &n.draft.Name, "例如：生产环境官网")
				a.ipNoteGroupPicker(c)
				ui.Text(c, "详细说明（可选）").FontSize(12)
				ui.TextArea(c.Key("ip-note-description"), &n.draft.Description).Height(100).Label("输入标注说明")
				ui.Text(c, "每个 IP 对应一条标注；无需填写端口。保存后相同远端 IP 的连接会立即显示标注。").FontSize(11).TextColor(c.Theme().TextMuted)
				if n.err != "" {
					ui.Text(c, n.err).TextColor(c.Theme().Warning)
				}
				if n.deleting {
					ui.Text(c, "确定删除这条 IP 标注？对应连接将恢复为未标注。").TextColor(c.Theme().Danger)
				}
			})
			ui.Row(c).Gap(10).Children(func() {
				if n.original != "" {
					if n.deleting {
						ui.Button(c, "确认删除标注").TextColor(c.Theme().Danger).OnClick(a.deleteIPNote)
						ui.Button(c, "保留标注").OnClick(func() { n.deleting = false })
					} else {
						ui.Button(c, "删除此标注").TextColor(c.Theme().Danger).OnClick(func() { n.deleting = true })
					}
				}
				ui.Spacer(c)
				ui.PrimaryButton(c, "保存 IP 标注").OnClick(a.saveIPNote)
			})
		})
	})
}

func (a *app) ipNoteGroupPicker(c *ui.Context) {
	n := &a.ipNotes
	ui.Row(c).Gap(14).Children(func() {
		ui.Text(c, "标注分组").Width(100).Shrink(0)
		ui.TextInput(c.Key("标注分组"), &n.draft.Group).Grow(1).MinWidth(0).Label("输入 标注分组").Placeholder("输入新分组，或点击下方已有分组")
		ui.Button(c, "清空分组").Disabled(n.draft.Group == "").OnClick(func() { n.draft.Group = "" })
	})
	groups := a.ipNoteGroups()
	if len(groups) == 0 {
		return
	}
	ui.Column(c.Key("saved-ip-groups")).Padding(0, 0, 0, 114).Gap(7).Children(func() {
		ui.Text(c, "已有分组 · 点击选用").FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for _, group := range groups {
				selected := strings.TrimSpace(n.draft.Group) == group
				text := group
				if selected {
					text = "✓ " + group
				}
				b := ui.Button(c.Key(group), text).FontSize(11).MaxWidth(450).Label("选用分组 " + group).Tooltip(group)
				if selected {
					b.TextColor(purple).Background(purple.Alpha(0.12))
				}
				b.OnClick(func() { n.draft.Group = group })
			}
		})
	})
}
