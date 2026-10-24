package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/index"
)

// Reproduce launching deja.exe directly, not through the plugin's configured
// child environment. The resulting index must reach the actual Agent picker.
func TestTUIDefaultDesktopStoreVisible(t *testing.T) {
	tmp := hermeticEnv(t)
	t.Setenv("DEJA_STORES", "pi-desktop")
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "")
	t.Setenv("PI_DESKTOP_DATA_DIR", "")
	root := filepath.Join(os.Getenv("USERPROFILE"), ".pi-desktop")
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	query := `CREATE TABLE projects(id INTEGER PRIMARY KEY,path TEXT); CREATE TABLE sessions(id TEXT PRIMARY KEY,title TEXT,project_id INTEGER,deleted_at INTEGER); INSERT INTO projects VALUES(1,'/work/cache'); INSERT INTO sessions VALUES('desktop-demo','Cache decision',1,NULL);`
	cmd := exec.Command("sqlite3", filepath.Join(root, "pi.sqlite"))
	cmd.Stdin = strings.NewReader(query)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed SQLite: %v %s", err, out)
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "registry", "pi-desktop", "desktop-demo.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sessions", "desktop-demo.jsonl"), body, 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "desktop-index")
	if err := index.Ensure(dir, "", false, io.Discard); err != nil {
		t.Fatal(err)
	}
	a := newTestTUI(t, dir)
	if len(a.allMeta) != 1 || a.allMeta[0].Harness != "pi-desktop" {
		t.Fatalf("standalone index omitted Desktop sessions: %+v", a.allMeta)
	}
	a.openModal(modalAgents)
	wantOnScreen(t, screen(a.frame(140, 36)), "PI-Desktop", "44 supported", "1 with sessions here")
	if len(a.m.ids) != 1 || a.m.ids[0] != "pi-desktop" {
		t.Fatalf("picker IDs: %v", a.m.ids)
	}
	// Capture the same screen buffer the terminal uses, without a live provider
	// or the developer's private history.
	t.Log("Synthetic Desktop Agent picker:\n" + screen(a.frame(140, 24)))
}
