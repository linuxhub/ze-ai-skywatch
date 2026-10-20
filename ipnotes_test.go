package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/ipnotes"
	"github.com/linuxhub/ze-ai-skywatch/internal/settings"
)

func TestIPNoteGroupReuse(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	a.configPath = filepath.Join(t.TempDir(), "settings.json")
	a.config.IPNotes = []ipnotes.Note{
		{IP: "10.0.0.1", Name: "官网", Group: "生产环境"},
		{IP: "10.0.0.2", Name: "网关", Group: " 生产环境 "},
		{IP: "10.0.0.3", Name: "测试", Group: "测试环境"},
		{IP: "10.0.0.4", Name: "未分组", Group: " "},
	}
	if got := a.ipNoteGroups(); !reflect.DeepEqual(got, []string{"生产环境", "测试环境"}) {
		t.Fatal(got)
	}
	a.page, a.ipNotes.showSettings = "settings", true
	a.editIPNote("10.0.0.1")
	tt := ui.NewTester(a.view, 1180, 700)
	tt.SetLocale("zh-Hans")
	click := func(s string) {
		t.Helper()
		if err := tt.Click(s); err != nil {
			t.Fatal(err)
		}
	}
	click("选用分组 测试环境")
	if a.ipNotes.draft.Group != "测试环境" || !tt.HasText("✓ 测试环境") {
		t.Fatal("group click/highlight failed")
	}
	click("清空分组")
	if a.ipNotes.draft.Group != "" || tt.HasText("✓ 测试环境") {
		t.Fatal("clear failed")
	}
	click("输入 标注分组")
	tt.Type("办公网络")
	if a.ipNotes.draft.Group != "办公网络" {
		t.Fatal("typing after selection failed")
	}
	click("保存 IP 标注")
	s, err := settings.Load(a.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if s.IPNotes[len(s.IPNotes)-1].Group != "办公网络" {
		t.Fatal("typed group did not persist")
	}
	click("添加 IP 标注")
	click("选用分组 办公网络")
	if a.ipNotes.draft.Group != "办公网络" {
		t.Fatal("new saved group not reusable")
	}
	click("取消标注编辑")
	click("返回常规设置")
	click("IP 标注簿")
	click("添加 IP 标注")
	click("选用分组 生产环境")
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if a.ipNotes.draft.Group != "生产环境" || tt.FramePending() {
		t.Fatal("picker lost identity across modal/page changes")
	}
}

func fixtureIPNotes(a *app, count int) {
	a.page, a.ipNotes.showSettings = "settings", true
	for i := 0; i < count; i++ {
		a.config.IPNotes = append(a.config.IPNotes, ipnotes.Note{
			IP: fmt.Sprintf("10.1.%d.%d", i/250, i%250+1), Name: fmt.Sprintf("业务服务器 %02d", i),
			Group: []string{"生产环境", "测试环境", "办公网络"}[i%3],
		})
	}
}

func TestIPNotesListFillsWindowAndKeepsHeader(t *testing.T) {
	a := fixtureApp()
	defer a.cancel()
	fixtureIPNotes(a, 80)
	tt := ui.NewTester(a.view, 1180, 700)
	tt.SetLocale("zh-Hans")
	table, ok := tt.Find("IP 标注列表")
	if !ok || table.H < 300 {
		t.Fatalf("table too short: %+v", table)
	}
	smallHeight := table.H
	tt.SetSize(1360, 1000)
	table, _ = tt.Find("IP 标注列表")
	if table.H < smallHeight+290 {
		t.Fatalf("table did not grow with window: %v -> %+v", smallHeight, table)
	}
	search, _ := tt.Find("搜索 IP 标注")
	header, _ := tt.Find("IP 地址")
	footer, _ := tt.Find("共 80 条 · 当前筛选 80 条")
	tt.Scroll(table.X+table.W/2, table.Y+table.H/2, 0, 800)
	if searchAfter, _ := tt.Find("搜索 IP 标注"); searchAfter != search {
		t.Fatal("search moved with table scroll")
	}
	if headerAfter, _ := tt.Find("IP 地址"); headerAfter != header {
		t.Fatal("header moved with table scroll")
	}
	if footerAfter, _ := tt.Find("共 80 条 · 当前筛选 80 条"); footerAfter != footer {
		t.Fatal("footer moved with table scroll")
	}
	if err := tt.Click("搜索 IP 标注"); err != nil {
		t.Fatal(err)
	}
	tt.Type("业务服务器 79")
	if !tt.HasText("共 80 条 · 当前筛选 1 条") || !tt.HasText("业务服务器 79") {
		t.Fatal("filter/count failed after scrolling")
	}
	if err := tt.Click("编辑"); err != nil {
		t.Fatal(err)
	}
	if a.ipNotes.draft.Name != "业务服务器 79" {
		t.Fatal("wrong filtered row edited")
	}
}

func TestIPNotesAdaptiveRender(t *testing.T) {
	for _, size := range [][2]int{{1180, 700}, {1360, 850}, {1900, 1200}} {
		for _, dark := range []bool{false, true} {
			a := fixtureApp()
			fixtureIPNotes(a, 60)
			tt := ui.NewTester(a.view, size[0], size[1])
			tt.SetLocale("zh-Hans")
			tt.SetDark(dark)
			for _, mode := range []string{"list", "groups"} {
				if mode == "groups" {
					for i := 0; i < 10; i++ {
						a.config.IPNotes[i].Group = fmt.Sprintf("应用集群分组 %02d", i)
					}
					a.editIPNote("10.1.0.1")
				}
				tt.Frame()
				if tt.FramePending() {
					t.Fatalf("view did not settle: %s", mode)
				}
				if mode == "groups" {
					button, ok := tt.Find("保存 IP 标注")
					if !ok || button.Y+button.H > float32(size[1]) {
						t.Fatal("save button clipped")
					}
				}
				if dir := os.Getenv("MAC_MONITOR_SCREENSHOTS"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("ipnotes-%s-%dx%d-%t.png", mode, size[0], size[1], dark)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, tt.Image())
					_ = f.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a.cancel()
		}
	}
}
