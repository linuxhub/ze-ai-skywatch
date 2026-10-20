package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/ipnotes"
	"github.com/linuxhub/ze-ai-skywatch/internal/settings"
)

type ipNotesState struct {
	showSettings      bool
	index             map[string]ipnotes.Note
	revision          uint64
	query             string
	list              ui.ListState
	editing, deleting bool
	original          string
	draft             ipnotes.Note
	err               string
}

// Reuse saved groups without a separate registry or a settings migration.
// Trim legacy values too, and rank frequently used groups first.
func (a *app) ipNoteGroups() []string {
	counts := map[string]int{}
	for _, note := range a.config.IPNotes {
		if group := strings.TrimSpace(note.Group); group != "" {
			counts[group]++
		}
	}
	groups := make([]string, 0, len(counts))
	for group := range counts {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if counts[groups[i]] != counts[groups[j]] {
			return counts[groups[i]] > counts[groups[j]]
		}
		return groups[i] < groups[j]
	})
	return groups
}

func (a *app) ipNote(ip string) ipnotes.Note {
	n := &a.ipNotes
	if n.index == nil {
		n.index = map[string]ipnotes.Note{}
		for _, note := range a.config.IPNotes {
			clean, err := ipnotes.Validate(note)
			if err == nil {
				if _, ok := n.index[clean.IP]; !ok {
					n.index[clean.IP] = clean
				}
			}
		}
	}
	return n.index[ipnotes.NormalizeIP(ip)]
}
func (a *app) noteSearch(ip string) string {
	n := a.ipNote(ip)
	return n.Name + " " + n.Group + " " + n.Description
}
func (a *app) editIPNote(ip string) {
	note := a.ipNote(ip)
	original := note.IP
	if original == "" {
		note.IP = ipnotes.NormalizeIP(ip)
	}
	a.ipNotes.editing, a.ipNotes.deleting = true, false
	a.ipNotes.original, a.ipNotes.draft, a.ipNotes.err = original, note, ""
}
func (a *app) commitIPNotes(notes []ipnotes.Note) error {
	next := a.config
	next.IPNotes = notes
	if a.configPath != "" {
		if err := settings.Save(a.configPath, next); err != nil {
			return fmt.Errorf("保存 IP 标注失败：%w", err)
		}
	}
	a.config = next
	a.workerConfig.Store(next)
	a.ipNotes.index = nil
	a.ipNotes.revision++
	return nil
}
func (a *app) saveIPNote() {
	n := &a.ipNotes
	note, err := ipnotes.Validate(n.draft)
	if err != nil {
		n.err = err.Error()
		return
	}
	notes := make([]ipnotes.Note, 0, len(a.config.IPNotes)+1)
	for _, old := range a.config.IPNotes {
		ip := ipnotes.NormalizeIP(old.IP)
		if ip == note.IP && ip != n.original {
			n.err = "这个 IP 已有标注，请编辑原记录"
			return
		}
		if ip != n.original || n.original == "" {
			notes = append(notes, old)
		}
	}
	if len(notes) >= 4096 {
		n.err = "最多保存 4096 条 IP 标注"
		return
	}
	notes = append(notes, note)
	if err := a.commitIPNotes(notes); err != nil {
		n.err = err.Error()
		return
	}
	n.editing = false
	a.notify("已保存 IP 标注：" + note.Name)
}
func (a *app) deleteIPNote() {
	n := &a.ipNotes
	notes := make([]ipnotes.Note, 0, len(a.config.IPNotes))
	for _, old := range a.config.IPNotes {
		if ipnotes.NormalizeIP(old.IP) != n.original {
			notes = append(notes, old)
		}
	}
	if err := a.commitIPNotes(notes); err != nil {
		n.err = err.Error()
		return
	}
	n.editing = false
	a.notify("已删除 IP 标注")
}
func (a *app) filteredIPNotes() []ipnotes.Note {
	q := strings.ToLower(strings.TrimSpace(a.ipNotes.query))
	rows := []ipnotes.Note{}
	for _, n := range a.config.IPNotes {
		if q == "" || strings.Contains(strings.ToLower(n.IP+" "+n.Name+" "+n.Group+" "+n.Description), q) {
			rows = append(rows, n)
		}
	}
	return rows
}
