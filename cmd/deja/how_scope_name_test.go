package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scope line names the project the way the rows do. Claude's encoding
// reads a hyphen as a slash, so for ~/proj/my-app the footer said "in my/app
// only", a name no row shows and --project does not take (#5321).
func TestTheScopeLineNamesAHyphenatedProjectLikeTheRows(t *testing.T) {
	tmp := hermeticEnv(t)
	t.Setenv("DEJA_INDEX_DIR", filepath.Join(tmp, "index.db"))
	cwd := filepath.Join(tmp, "proj", "my-app")
	if err := os.MkdirAll(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cwdJSON, err := json.Marshal(cwd)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := json.Marshal(filepath.Join(cwd, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("DEJA_CLAUDE_ROOT")
	for _, id := range []string{"s1", "s2"} {
		writeClaudeFixture(t, filepath.Join(root, claudeEncodedDirForTest(cwd), id+".jsonl"), id, []string{
			`{"type":"user","sessionId":"` + id + `","cwd":` + string(cwdJSON) + `,"timestamp":"2026-10-17T10:00:00Z","message":{"role":"user","content":"write the release notes"}}`,
			`{"type":"assistant","sessionId":"` + id + `","cwd":` + string(cwdJSON) + `,"timestamp":"2026-10-17T10:01:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{"file_path":` + string(notes) + `,"content":"release"}}]}}`,
		})
	}
	if _, err = captureRun(t, "index"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	if got := howScopeName(howScope(cwd, "", false)); got != "proj/my-app" {
		t.Errorf("scope name = %q, want proj/my-app", got)
	}
	out, err := captureRun(t, "files", "release")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "notes.txt") {
		t.Fatalf("the fixture no longer answers, so this pins nothing:\n%s", out)
	}
	if !strings.Contains(out, "in proj/my-app only") {
		t.Errorf("the footer does not name the project the rows do:\n%s", out)
	}
}

// claudeEncodedDirForTest is the directory name Claude Code keeps a
// project's transcripts under: the path with every separator, dot and colon
// turned into a dash.
func claudeEncodedDirForTest(dir string) string {
	return strings.NewReplacer("/", "-", "\\", "-", ".", "-", ":", "-").Replace(dir)
}
