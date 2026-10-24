package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Each install and uninstall below is a deja process of its own: what a run
// created is recorded per process, and carried over, a second install would skip
// the snapshot that a real second run takes.
func anotherRun() {
	createdDirsByThisRun, createdByThisRun, snapshotsByThisRun = nil, nil, nil
	blocksAddedThisRun, blocksForgottenThisRun = nil, map[string]bool{}
}

// A file deja owns whole embeds the binary path. A reinstall from another path
// rewrites it and writeIfChanged snapshots deja's previous text as a .bak; uninstall
// removed the file but left that snapshot and reported it as the reader's (#5299).
func TestUninstallDropsTheSnapshotOfDejasOwnSteeringText(t *testing.T) {
	hermeticEnv(t)
	first := filepath.Join(t.TempDir(), "bin-one", "deja")
	second := filepath.Join(t.TempDir(), "bin-two", "deja")
	for _, exe := range []string{first, second} {
		anotherRun()
		if _, err := installKiro(exe, false); err != nil {
			t.Fatal(err)
		}
	}
	path := kiroSteeringPath()
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("setup: the reinstall from another path should have snapshotted the old text: %v", err)
	}
	anotherRun()
	if _, err := installKiro(second, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("deja's steering file survived uninstall: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("deja's own text was kept as a snapshot of the reader's config: %v", err)
	}
}

// The other side of the same rule: a steering file the reader wrote is replaced by
// install, and uninstall puts it back rather than deleting their snapshot.
func TestUninstallPutsBackTheSteeringFileItReplaced(t *testing.T) {
	hermeticEnv(t)
	path := kiroSteeringPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "# my own steering\nAlways answer in French.\n"
	if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "deja")
	anotherRun()
	if _, err := installKiro(exe, false); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path + ".bak"); err != nil || string(b) != mine {
		t.Fatalf("install kept no copy of the file it replaced: %v", err)
	}
	anotherRun()
	if _, err := installKiro(exe, true); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != mine {
		t.Errorf("the reader's steering file was not put back: %v\n%s", err, b)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("the copy was put back and the snapshot left behind: %v", err)
	}
}

func TestSameTextExceptExe(t *testing.T) {
	body := "intro\nrun /new/bin/deja search -- \"q\"\noutro\n"
	for _, c := range []struct {
		name, other string
		want        bool
	}{
		{"identical", body, true},
		{"another binary path", "intro\nrun /old/bin/deja search -- \"q\"\noutro\n", true},
		{"CRLF copy", "intro\r\nrun /old/deja search -- \"q\"\r\noutro\r\n", true},
		{"a reader's edit to another line", "intro\nrun /old/bin/deja search -- \"q\"\nmy note\n", false},
		{"a reader's edit to the exe line itself", "intro\nrun /old/bin/deja find -- \"q\"\noutro\n", false},
		{"a line added", body + "extra\n", false},
	} {
		if got := sameTextExceptExe(body, c.other, "/new/bin/deja"); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if sameTextExceptExe(body, "intro\nrun /old/bin/deja search -- \"q\"\noutro\n", "") {
		t.Error("without an exe only an identical text is deja's")
	}
}
