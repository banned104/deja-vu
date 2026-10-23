package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
	"github.com/vshulcz/deja-vu/internal/tui"
)

func TestPathsCLIAndTUIUseSameSettings(t *testing.T) {
	tmp := hermeticEnv(t)
	t.Setenv("DEJA_STORES", "")
	selected := filepath.Join(tmp, "moved-data")
	var out bytes.Buffer
	if err := runPaths(&out, []string{"set", "pi-desktop", "data", selected}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Index unchanged") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := runPaths(&out, []string{"show", "pi-desktop", "--json"}); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Fields []sources.SourcePathInfo `json:"fields"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Fields) != 1 || report.Fields[0].Origin != "user" || report.Fields[0].Paths[0] != selected {
		t.Fatal(out.String())
	}
	a := newTUIApp(filepath.Join(tmp, "index"), nil)
	t.Cleanup(a.bg.Wait)
	a.handle(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: 'P'})
	if a.modal != modalPaths {
		t.Fatal("P did not open settings")
	}
	a.pathSettings.query = []rune("pi-desktop")
	a.pathSettings.selected = 0
	// Long temporary paths are clipped on screen; the shared value must remain intact.
	fields := a.visiblePaths()
	if len(fields) != 1 || len(fields[0].Paths) != 1 || fields[0].Paths[0] != selected {
		t.Fatalf("TUI did not load the CLI path: %+v", fields)
	}
	wantOnScreen(t, screen(a.frame(150, 36)), "Agent history paths", "PI-Desktop.data", "user")
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: ' '})
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: 'y'})
	if !sources.StoreSilenced("pi-desktop") {
		t.Fatal("TUI did not disable source")
	}
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: ' '})
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: 'y'})
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyEnter})
	if !a.pathSettings.editing {
		t.Fatal("Enter did not edit")
	}
	if string(a.pathSettings.text) != selected {
		t.Fatalf("editor lost the full path: %q", string(a.pathSettings.text))
	}
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyCtrl, Rune: 'u'})
	updated := filepath.Join(tmp, "updated-data")
	a.handlePaths(tui.Event{Kind: tui.EvPaste, Text: updated})
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyEnter})
	if a.pathSettings.editing || sources.PiDesktopRoot() != updated {
		t.Fatalf("save failed: %s", a.pathSettings.message)
	}
	if _, err := os.Stat(a.dir); !os.IsNotExist(err) {
		t.Fatal("editing built index without confirmation")
	}
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: 'i'})
	if !a.pathSettings.confirmIndex {
		t.Fatal("missing rebuild confirmation")
	}
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: 'n'})
	if a.indexing {
		t.Fatal("declined rebuild started")
	}
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: 'd'})
	if sources.PiDesktopRoot() != updated {
		t.Fatal("reset applied before confirmation")
	}
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyRune, Rune: 'y'})
	if sources.PiDesktopRoot() == updated {
		t.Fatal("automatic reset failed")
	}
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyEsc})
	if a.modal != modalNone {
		t.Fatal("settings did not close")
	}
}
func TestPathsCLIRejectsInvalidCommands(t *testing.T) {
	hermeticEnv(t)
	for _, args := range [][]string{{"unknown"}, {"set"}, {"disable", "bad-agent"}, {"set", "pi-desktop", "data", "relative"}, {"show", "bad-agent"}, {"list", "extra"}} {
		if err := runPaths(&bytes.Buffer{}, args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestPathsShortTerminalAndEmptySearch(t *testing.T) {
	tmp := hermeticEnv(t)
	a := newTUIApp(filepath.Join(tmp, "index"), nil)
	t.Cleanup(a.bg.Wait)
	a.openPaths()
	a.pathSettings.query = []rune("pi-desktop")
	wantOnScreen(t, screen(a.frame(120, 24)), "PI-Desktop.data", "Enter/e Edit")
	a.pathSettings.query = []rune("nothing-matches")
	a.pathSettings.selected = 20
	a.handlePaths(tui.Event{Kind: tui.EvKey, Key: tui.KeyEnter})
	if a.pathSettings.editing {
		t.Fatal("editing nonexistent field")
	}
	_ = a.frame(80, 18)
}
