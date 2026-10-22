package ai

import (
	"strings"
	"testing"
)

func TestParseConnectionReport(t *testing.T) {
	valid := `{"summary":"固定快照","application":{"name":"示例应用（推测）","company":"未知"},"connection":"仅连接元数据","route":"按规则推断","findings":["1","2","3","4","5"]}`
	for _, text := range []string{valid, "```json\n" + valid + "\n```"} {
		report, err := ParseConnectionReport(text)
		if err != nil || report.Summary != "固定快照" || len(report.Findings) != 4 {
			t.Fatal(report, err)
		}
	}
	for _, text := range []string{"# 原文", `{}`, `{"summary":"x"}`, strings.Repeat("x", 100*1024)} {
		if _, err := ParseConnectionReport(text); err == nil {
			t.Fatal("invalid structured report accepted")
		}
	}
}
