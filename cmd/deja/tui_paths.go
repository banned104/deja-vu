package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/vshulcz/deja-vu/internal/index"
	"github.com/vshulcz/deja-vu/internal/sources"
	"github.com/vshulcz/deja-vu/internal/tui"
)

type pathModalState struct {
	fields                                  []sources.SourcePathInfo
	query                                   []rune
	selected                                int
	editing, searching, confirmIndex, dirty bool
	text                                    []rune
	message                                 string
	confirmAction                           func() error
	confirmText                             string
}

func (a *tuiApp) openPaths() {
	a.modal = modalPaths
	a.pathSettings = pathModalState{}
	a.reloadPathSettings()
}
func (a *tuiApp) reloadPathSettings() {
	fields, err := sources.SourcePathInfos()
	if err != nil {
		a.pathSettings.message = err.Error()
		return
	}
	a.pathSettings.fields = fields
	a.pathSettings.selected = max(0, min(a.pathSettings.selected, len(a.visiblePaths())-1))
}
func (a *tuiApp) visiblePaths() []sources.SourcePathInfo {
	var out []sources.SourcePathInfo
	q := strings.ToLower(string(a.pathSettings.query))
	for _, field := range a.pathSettings.fields {
		if q == "" || strings.Contains(strings.ToLower(agentName(field.Agent)+" "+field.Agent+" "+field.Key+" "+field.Label), q) {
			out = append(out, field)
		}
	}
	return out
}
func (a *tuiApp) handlePaths(ev tui.Event) {
	p := &a.pathSettings
	p.selected = max(0, min(p.selected, len(a.visiblePaths())-1))
	if a.indexing && !(ev.Kind == tui.EvKey && ev.Key == tui.KeyEsc) {
		p.message = "Index update in progress; wait before changing source paths."
		return
	}
	if ev.Kind == tui.EvPaste && (p.editing || p.searching) {
		if p.editing {
			p.text = append(p.text, []rune(strings.ReplaceAll(ev.Text, "\r\n", "\n"))...)
		} else {
			p.query = append(p.query, []rune(strings.Join(strings.Fields(ev.Text), " "))...)
			p.selected = 0
		}
		return
	}
	if ev.Kind != tui.EvKey {
		return
	}
	if p.confirmAction != nil {
		action := p.confirmAction
		p.confirmAction = nil
		if ev.Key == tui.KeyRune && (ev.Rune == 'y' || ev.Rune == 'Y') {
			if err := action(); err != nil {
				p.message = err.Error()
			} else {
				p.dirty = true
				p.message = "Saved; index unchanged."
				a.reloadPathSettings()
			}
		} else {
			p.message = "Cancelled; settings unchanged."
		}
		return
	}
	if p.confirmIndex {
		p.confirmIndex = false
		if ev.Key == tui.KeyRune && (ev.Rune == 'y' || ev.Rune == 'Y') {
			a.reindexFromPaths()
		}
		return
	}
	if p.editing || p.searching {
		switch ev.Key {
		case tui.KeyEsc:
			p.editing = false
			p.searching = false
		case tui.KeyCtrl:
			if ev.Rune == 'u' {
				if p.editing {
					p.text = nil
				} else {
					p.query = nil
				}
			}
		case tui.KeyBackspace:
			if p.editing && len(p.text) > 0 {
				p.text = p.text[:len(p.text)-1]
			}
			if p.searching && len(p.query) > 0 {
				p.query = p.query[:len(p.query)-1]
				p.selected = 0
			}
		case tui.KeyRune:
			if p.editing {
				p.text = append(p.text, ev.Rune)
			} else {
				p.query = append(p.query, ev.Rune)
				p.selected = 0
			}
		case tui.KeyEnter:
			if p.searching {
				p.searching = false
				return
			}
			fields := a.visiblePaths()
			if p.selected >= len(fields) {
				return
			}
			field := fields[p.selected]
			var paths []string
			for _, line := range strings.Split(strings.TrimSpace(string(p.text)), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					paths = append(paths, line)
				}
			}
			if err := sources.SetSourcePath(field.Agent, field.Key, paths); err != nil {
				p.message = err.Error()
				return
			}
			p.editing = false
			p.dirty = true
			p.message = "Saved; index unchanged. Press i and confirm when ready to update."
			a.reloadPathSettings()
		}
		return
	}
	fields := a.visiblePaths()
	switch ev.Key {
	case tui.KeyEsc:
		a.modal = modalNone
		return
	case tui.KeyUp:
		p.selected--
	case tui.KeyDown, tui.KeyTab:
		p.selected++
	case tui.KeyPgUp:
		p.selected -= 8
	case tui.KeyPgDn:
		p.selected += 8
	case tui.KeyEnter:
		p.editing = true
	case tui.KeyRune:
		switch ev.Rune {
		case '/':
			p.searching = true
			return
		case 'e':
			p.editing = true
		case 'i':
			if !a.indexing {
				p.confirmIndex = true
			}
			return
		case 'd', ' ', 'c':
			if len(fields) == 0 {
				return
			}
			field := fields[p.selected]
			switch ev.Rune {
			case 'd':
				p.confirmText = "Restore automatic path for " + agentName(field.Agent) + "." + field.Key + " only? y / any other key cancels"
				p.confirmAction = func() error { return sources.ResetSourcePathField(field.Agent, field.Key) }
			case ' ':
				p.confirmText = "Enable/disable ALL paths for " + agentName(field.Agent) + "? y / any other key cancels"
				p.confirmAction = func() error { return sources.SetSourceEnabled(field.Agent, !field.Enabled) }
			case 'c':
				p.message = "Checking source files (not conversation content)…"
				a.spawn(func() {
					n, err := sources.CheckSource(field.Agent)
					a.post(func() {
						if err != nil {
							a.pathSettings.message = err.Error()
						} else {
							a.pathSettings.message = fmt.Sprintf("%s: %d source files; no content read, no index written.", agentName(field.Agent), n)
						}
					})
				})
			}
			a.reloadPathSettings()
		}
	}
	p.selected = max(0, min(p.selected, len(fields)-1))
	if p.editing && len(fields) > 0 {
		p.text = []rune(strings.Join(fields[p.selected].Paths, "\n"))
		p.message = "Enter saves · Esc cancels · Ctrl+U clears · paste multiple directories on separate lines"
	} else {
		p.editing = false
	}
}
func (a *tuiApp) reindexFromPaths() {
	if a.indexing {
		return
	}
	if err := sources.SourcePathSettingsError(); err != nil {
		a.pathSettings.message = err.Error()
		return
	}
	a.indexing = true
	a.pathSettings.message = "Updating index…"
	a.spawn(func() {
		err := index.Ensure(a.dir, "", true, io.Discard)
		a.post(func() {
			a.indexing = false
			if err != nil {
				a.pathSettings.message = "Index update failed: " + err.Error()
				return
			}
			a.pathSettings.dirty = false
			a.pathSettings.message = "Index updated."
			a.allStamp = ""
			a.reload()
			a.reloadPathSettings()
		})
	})
}
func (a *tuiApp) drawPaths() {
	p := &a.pathSettings
	count := max(1, min(9, a.p.H-19))
	x, y, w := a.modalBox(116, count+12)
	right := x + w
	a.p.PutClip(x, y, "Agent history paths · "+num(len(agentNames))+" agents · shared by CLI / TUI / MCP", bold(cText), right)
	a.p.PutClip(x, y+1, sources.SourcePathsFile(), fgs(cMuted), right)
	query := "/ Search: " + string(p.query)
	if p.searching {
		query += "▏"
	}
	a.p.PutClip(x, y+2, query, fgs(cText), right)
	fields := a.visiblePaths()
	// Keep the selected row and action hints visible in short terminals.
	start := max(0, p.selected-count+1)
	for i := start; i < len(fields) && i < start+count; i++ {
		field := fields[i]
		row := y + 4 + i - start
		state := "on"
		if !field.Enabled {
			state = "off"
		} else if !field.Read {
			state = "excluded"
		}
		label := fmt.Sprintf("%-22s %-11s %-11s %-8s %s", agentName(field.Agent)+"."+field.Key, field.Kind, field.Origin, state, field.Status)
		st := fgs(cMuted)
		if i == p.selected {
			st = bold(cText)
			a.p.Fill(x, row, w, 1, cOver)
		}
		a.p.PutClip(x, row, label, st, right)
	}
	bottom := y + 5 + count
	if len(fields) > 0 {
		selected := fields[max(0, min(p.selected, len(fields)-1))]
		a.p.PutClip(x, bottom, selected.Label+" · "+selected.Env, fgs(cMuted), right)
		path := strings.Join(selected.Paths, " | ")
		if p.editing {
			path = "> " + strings.ReplaceAll(string(p.text), "\n", " | ") + "▏"
		}
		a.p.PutClip(x, bottom+1, path, fgs(cText), right)
	}
	message := p.message
	if p.confirmIndex {
		message = "Update the index using ALL currently enabled sources now? y / any other key cancels"
	}
	if p.confirmAction != nil {
		message = p.confirmText
	}
	a.p.PutClip(x, bottom+3, message, fgs(cText), right)
	a.p.PutClip(x, bottom+4, "Enter/e Edit · c Check · d Automatic · space Enable/disable · / Search · i Update index · Esc Close", fgs(cMuted), right)
}
