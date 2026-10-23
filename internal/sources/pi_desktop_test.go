package sources

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func desktopFixture(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("DEJA_STORES", "pi-desktop")
	root := filepath.Join(home, "desktop")
	t.Setenv("DEJA_PI_DESKTOP_ROOT", root)
	t.Setenv("PI_DESKTOP_DATA_DIR", "")
	t.Setenv("DEJA_SOURCE_PATHS_FILE", filepath.Join(home, "source-paths.json"))
	if !SQLite3Available() {
		t.Fatal("sqlite3 required for Desktop fixture")
	}
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	desktopSQL(t, root, `CREATE TABLE projects(id INTEGER PRIMARY KEY,path TEXT); CREATE TABLE sessions(id TEXT PRIMARY KEY,title TEXT,project_id INTEGER,deleted_at INTEGER); INSERT INTO projects VALUES(1,'/work/cache'); INSERT INTO sessions VALUES('desktop-demo','Cache decision',1,NULL);`)
	b, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "registry", "pi-desktop", "desktop-demo.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sessions", "desktop-demo.jsonl")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	return root, path
}
func desktopSQL(t *testing.T, root, sql string) {
	t.Helper()
	if b, err := exec.Command("sqlite3", filepath.Join(root, "pi.sqlite"), sql).CombinedOutput(); err != nil {
		t.Fatalf("fixture sql: %v %s", err, b)
	}
}

func TestPiDesktopReadsCanonicalBlocks(t *testing.T) {
	_, path := desktopFixture(t)
	ss, err := ParsePiDesktopFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 1 {
		t.Fatalf("sessions = %v", ss)
	}
	s := ss[0]
	if s.ID != "desktop-demo" || s.Harness != "pi-desktop" || s.Project != "work/cache" || s.Title != "Cache decision" {
		t.Fatalf("identity: %+v", s)
	}
	var all strings.Builder
	roles := map[string]bool{}
	for _, m := range s.Messages {
		all.WriteString(m.Text + "\n")
		roles[m.Role] = true
	}
	for _, want := range []string{"quartz-token", "go test ./cache  → exit 1", "/work/cache/cache/epoch.go"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("missing %q in %s", want, all.String())
		}
	}
	if strings.Contains(all.String(), "PRIVATE_REASONING") {
		t.Fatal("indexed thinking")
	}
	for _, role := range []string{"user", "assistant", RoleCommand, RoleFiles, RoleSummary, RoleToolOutput} {
		if !roles[role] {
			t.Errorf("missing role %s", role)
		}
	}
}

func TestPiDesktopDiscoveryIsFlat(t *testing.T) {
	root, path := desktopFixture(t)
	for _, name := range []string{"desktop-demo.revisions.jsonl", "desktop-demo.inflight.json"} {
		if err := os.WriteFile(filepath.Join(root, "sessions", name), []byte("{}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if files := PiDesktopSessionFiles(); len(files) != 1 || files[0] != path {
		t.Fatalf("files: %v", files)
	}
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "")
	if PiDesktopRoot() == root {
		t.Fatal("explicit override was not cleared")
	}
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "relative")
	if PiDesktopRoot() != "" {
		t.Fatal("relative root accepted")
	}
	t.Setenv("DEJA_PI_DESKTOP_ROOT", root)
	t.Setenv("DEJA_STORES", "notes")
	if PiDesktopRoot() != "" {
		t.Fatal("store selection bypassed")
	}
}

func TestPiDesktopMetadataChangesAndDeletion(t *testing.T) {
	root, path := desktopFixture(t)
	a, b := piDesktopSidecar(path)
	desktopSQL(t, root, `UPDATE sessions SET title='New longer title'; UPDATE projects SET path='/work/other';`)
	c, d := piDesktopSidecar(path)
	if a == c && b == d {
		t.Fatal("metadata fingerprint unchanged")
	}
	ss, err := ParsePiDesktopFile(path)
	if err != nil || len(ss) != 1 || ss[0].Title != "New longer title" || ss[0].Project != "work/other" {
		t.Fatalf("refresh: %v %v", ss, err)
	}
	if PiDesktopWorkspace(path) != "/work/other" {
		t.Fatal("workspace not refreshed")
	}
	desktopSQL(t, root, `UPDATE sessions SET deleted_at=42;`)
	ss, err = ParsePiDesktopFile(path)
	if err != nil || len(ss) != 0 {
		t.Fatalf("deleted session read: %v %v", ss, err)
	}
	desktopSQL(t, root, `DELETE FROM sessions;`)
	ss, err = ParsePiDesktopFile(path)
	if err != nil || len(ss) != 0 {
		t.Fatalf("orphan read: %v %v", ss, err)
	}
}

func TestPiDesktopLastWriteWinsAndTornTail(t *testing.T) {
	_, path := desktopFixture(t)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("{\"type\":\"message\",\"id\":\"a1\",\"role\":\"assistant\",\"createdAt\":\"2026-10-01T09:01:00Z\",\"blocks\":[{\"type\":\"text\",\"text\":\"Replacement decision uses epochs.\"}]}\n{\"type\":")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	ss, err := ParsePiDesktopFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, m := range ss[0].Messages {
		text += m.Text + "\n"
	}
	if strings.Contains(text, "Decided to invalidate") || strings.Count(text, "Replacement decision") != 1 {
		t.Fatal(text)
	}
}

func TestPiDesktopRejectsUnknownSchemaAndCorruption(t *testing.T) {
	_, path := desktopFixture(t)
	for _, body := range []string{`{"type":"session","schema":2,"sessionId":"desktop-demo"}` + "\n", `{"type":"session","schema":1,"sessionId":"wrong"}` + "\n", `{"type":"session","schema":1,"sessionId":"desktop-demo"}` + "\nnot-json\n"} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ParsePiDesktopFile(path); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestPiDesktopResultKeepsBothStreams(t *testing.T) {
	if got := piDesktopResultText(map[string]any{"stdout": "build", "stderr": "failure"}); got != "build\nfailure" {
		t.Fatal(got)
	}
}

func TestPiDesktopRejectsUnselectedRootAndOversizedFile(t *testing.T) {
	root, path := desktopFixture(t)
	t.Setenv("DEJA_PI_DESKTOP_ROOT", filepath.Join(root, "not-the-selected-root"))
	if _, err := ParsePiDesktopFile(path); err == nil {
		t.Fatal("read without selected root")
	}
	t.Setenv("DEJA_PI_DESKTOP_ROOT", root)
	if err := os.Truncate(path, (64<<20)+1); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePiDesktopFile(path); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("large read: %v", err)
	}
}

func TestPiDesktopCompactionUsesNativeIdentityAndWorkspace(t *testing.T) {
	_, path := desktopFixture(t)
	capture, err := ReadCompactionSession("pi-desktop", "desktop-demo", "", "/not-the-project")
	if err != nil {
		t.Fatal(err)
	}
	if capture.Path != path || capture.Workspace != "/work/cache" || capture.NativeSessionID != "desktop-demo" {
		t.Fatalf("capture: %+v", capture)
	}
	if _, err := ReadCompactionSession("pi-desktop", "not-present", "", "/work/cache"); err == nil {
		t.Fatal("captured wrong session")
	}
}

func TestPiDesktopWALIsPartOfMetadataFingerprint(t *testing.T) {
	root, path := desktopFixture(t)
	a, b := piDesktopSidecar(path)
	if err := os.WriteFile(filepath.Join(root, "pi.sqlite-wal"), []byte("synthetic fingerprint only"), 0600); err != nil {
		t.Fatal(err)
	}
	c, d := piDesktopSidecar(path)
	if a == c && b == d {
		t.Fatal("WAL did not invalidate metadata")
	}
}

func TestPiDesktopStructuredToolResultKeepsTextAndExit(t *testing.T) {
	_, path := desktopFixture(t)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"type":"message","id":"structured","role":"tool","createdAt":"2026-10-01T09:03:00Z","blocks":[{"type":"tool_call","callId":"structured-call","name":"Bash","args":{"command":"go test ./structured"},"status":"error","result":{"content":[{"type":"text","text":"structured failure"}],"details":{"exitCode":7}}}]}` + "\n")
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	ss, err := ParsePiDesktopFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, message := range ss[0].Messages {
		text += message.Text + "\n"
	}
	if !strings.Contains(text, "structured failure") || !strings.Contains(text, "go test ./structured  → exit 7") {
		t.Fatal(text)
	}
}
