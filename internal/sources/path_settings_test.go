package sources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolatedPathSettings(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME"} {
		t.Setenv(key, home)
	}
	t.Setenv("PI_DESKTOP_DATA_DIR", "")
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "")
	t.Setenv("DEJA_STORES", "")
	path := filepath.Join(home, "config", "source-paths.json")
	t.Setenv("DEJA_SOURCE_PATHS_FILE", path)
	return home
}
func TestPathCatalogCoversEveryHarness(t *testing.T) {
	isolatedPathSettings(t)
	for _, h := range AllHarnesses() {
		if h.Name != "deja" && len(PathFields(h.Name)) == 0 {
			t.Errorf("no configurable fields for %s", h.Name)
		}
	}
	seen := map[string]bool{}
	for _, field := range pathFieldTable() {
		if seen[field.Env] {
			t.Errorf("duplicate env %s", field.Env)
		}
		seen[field.Env] = true
	}
}
func TestSourcePathsPersistAndEnvironmentWins(t *testing.T) {
	home := isolatedPathSettings(t)
	custom := filepath.Join(home, "moved")
	if err := SetSourcePath("pi-desktop", "data", []string{custom}); err != nil {
		t.Fatal(err)
	}
	if got := PiDesktopRoot(); got != custom {
		t.Fatalf("saved root = %s", got)
	}
	env := filepath.Join(home, "temporary")
	t.Setenv("DEJA_PI_DESKTOP_ROOT", env)
	if got := PiDesktopRoot(); got != env {
		t.Fatalf("environment lost: %s", got)
	}
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "")
	value, err := ReadSourcePathSettings()
	if err != nil {
		t.Fatal(err)
	}
	value.Agents["pi-desktop"].Paths["data"][0] = "mutated copy"
	if got := PiDesktopRoot(); got != custom {
		t.Fatalf("cache mutable: %s", got)
	}
	if err := ResetSourcePaths("pi-desktop"); err != nil {
		t.Fatal(err)
	}
	if got := PiDesktopRoot(); got != filepath.Join(home, ".pi-desktop") {
		t.Fatalf("reset = %s", got)
	}
}
func TestSourcePathsMissingSelectionDoesNotFallBack(t *testing.T) {
	home := isolatedPathSettings(t)
	missing := filepath.Join(home, "not-mounted")
	if err := SetSourcePath("claude", "sessions", []string{missing}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEJA_CLAUDE_ROOT", "")
	roots := ClaudeRoots()
	if len(roots) != 1 || roots[0] != missing {
		t.Fatalf("fell back to private store: %v", roots)
	}
	if _, err := CheckSource("claude"); err == nil {
		t.Fatal("missing custom root reported available")
	}
	for _, h := range AllHarnesses() {
		for _, f := range PathFields(h.Name) {
			if f.Kind == "directories" {
				continue
			}
			if err := SetSourcePath(h.Name, f.Key, []string{missing}); err != nil {
				t.Fatalf("%s.%s: %v", h.Name, f.Key, err)
			}
		}
	}
}
func TestSourcePathsListsAndDisable(t *testing.T) {
	home := isolatedPathSettings(t)
	a := filepath.Join(home, "a")
	b := filepath.Join(home, "b")
	t.Setenv("DEJA_COPILOT_CHAT_ROOTS", "")
	if err := SetSourcePath("copilot-chat", "roots", []string{a, b}); err != nil {
		t.Fatal(err)
	}
	roots := CopilotChatRoots()
	if len(roots) != 2 || roots[0] != a || roots[1] != b {
		t.Fatalf("roots %v", roots)
	}
	if err := SetSourceEnabled("pi-desktop", false); err != nil {
		t.Fatal(err)
	}
	if !StoreSilenced("pi-desktop") {
		t.Fatal("disabled source still active")
	}
	for _, h := range StoresToRead() {
		if h.Name == "pi-desktop" {
			t.Fatal("disabled source still walked")
		}
	}
	if err := SetSourceEnabled("pi-desktop", true); err != nil {
		t.Fatal(err)
	}
	if StoreSilenced("pi-desktop") {
		t.Fatal("enable failed")
	}
}
func TestSourcePathsValidateBeforeSaveAndFailClosed(t *testing.T) {
	home := isolatedPathSettings(t)
	for _, args := range [][3]string{{"no-agent", "data", home}, {"pi-desktop", "wrong", home}, {"pi-desktop", "data", "relative"}} {
		if err := SetSourcePath(args[0], args[1], []string{args[2]}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if err := SetSourcePath("pi-desktop", "data", []string{home, home}); err == nil {
		t.Fatal("accepted multiple single roots")
	}
	path := SourcePathsFile()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := SourcePathInfos(); err == nil {
		t.Fatal("bad config silently ignored")
	}
	if got := PiDesktopRoot(); got != "" {
		t.Fatalf("scanned default after bad config: %s", got)
	}
	if _, err := SetAndReadUnknownVersionForTest(path); err == nil {
		t.Fatal("unknown version accepted")
	}
}
func SetAndReadUnknownVersionForTest(path string) (SourcePathSettings, error) {
	if err := os.WriteFile(path, []byte(`{"version":2,"agents":{}}`), 0600); err != nil {
		return SourcePathSettings{}, err
	}
	return ReadSourcePathSettings()
}
func TestPiDesktopMigrationPointerAndUserPrecedence(t *testing.T) {
	home := isolatedPathSettings(t)
	moved := filepath.Join(home, "migrated")
	pointer := piDesktopPreferenceFile()
	if err := os.MkdirAll(filepath.Dir(pointer), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"version": 1, "roots": map[string]string{"data": moved}, "backups": []string{filepath.Join(home, "old")}})
	if err := os.WriteFile(pointer, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got := PiDesktopRoot(); got != moved {
		t.Fatalf("migration pointer ignored: %s", got)
	}
	user := filepath.Join(home, "selected")
	if err := SetSourcePath("pi-desktop", "data", []string{user}); err != nil {
		t.Fatal(err)
	}
	if got := PiDesktopRoot(); got != user {
		t.Fatalf("user selection lost: %s", got)
	}
	if err := ResetSourcePaths("pi-desktop"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pointer, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := PiDesktopRoot(); got != "" {
		t.Fatalf("bad pointer fell back: %s", got)
	}
}
func TestPathInfoJSONAndNoAutomaticIndex(t *testing.T) {
	home := isolatedPathSettings(t)
	infos, err := SourcePathInfos()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(infos)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"agent":"pi-desktop"`) {
		t.Fatal("missing source")
	}
	if _, err := os.Stat(filepath.Join(home, "index.db")); !os.IsNotExist(err) {
		t.Fatal("settings unexpectedly built index")
	}
}

func TestSourcePathsNullAgentsAndUnavailableSelection(t *testing.T) {
	home := isolatedPathSettings(t)
	path := SourcePathsFile()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"agents":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetSourcePath("pi-desktop", "data", []string{filepath.Join(home, "offline")}); err != nil {
		t.Fatal(err)
	}
	if err := SourcePathSettingsError(); err == nil {
		t.Fatal("unavailable custom path accepted for indexing")
	}
	if err := SetSourceEnabled("pi-desktop", false); err != nil {
		t.Fatal(err)
	}
	if err := SourcePathSettingsError(); err != nil {
		t.Fatalf("disabled offline source blocks indexing: %v", err)
	}
}

func TestSourcePathConcurrentUpdatesDoNotLoseOtherAgents(t *testing.T) {
	home := isolatedPathSettings(t)
	results := make(chan error, 2)
	go func() { results <- SetSourcePath("claude", "sessions", []string{filepath.Join(home, "claude")}) }()
	go func() { results <- SetSourcePath("pi-desktop", "data", []string{filepath.Join(home, "desktop")}) }()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	value, err := ReadSourcePathSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Agents) != 2 {
		t.Fatalf("lost concurrent update: %+v", value)
	}
}

func TestSourcePathFieldOriginsAndSinglePathDelimiter(t *testing.T) {
	home := isolatedPathSettings(t)
	selected := filepath.Join(home, "cache;data")
	t.Setenv("DEJA_PI_DESKTOP_ROOT", selected)
	fields, err := SourcePathInfos()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range fields {
		if field.Agent == "pi-desktop" {
			if len(field.Paths) != 1 || field.Paths[0] != selected || field.Origin != "environment" {
				t.Fatalf("bad single root: %+v", field)
			}
		}
	}
}

func TestSourcePathsEmptyDesktopAndLosslessLists(t *testing.T) {
	home := isolatedPathSettings(t)
	root := filepath.Join(home, "empty-desktop")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pi.sqlite"), []byte("metadata-only fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SetSourcePath("pi-desktop", "data", []string{root}); err != nil {
		t.Fatal(err)
	}
	if err := SourcePathSettingsError(); err != nil {
		t.Fatalf("empty valid data root blocks indexing: %v", err)
	}
	for _, test := range []struct {
		id, key string
		get     func() []string
	}{
		{"muse", "sessions", MuseRoots}, {"roo", "roots", RooRoots}, {"kilocode", "roots", KiloRoots}, {"copilot-chat", "roots", CopilotChatRoots}, {"codebuddy", "roots", CodeBuddyRoots}, {"cline", "roots", ClineLegacyRoots},
	} {
		paths := []string{filepath.Join(home, "one;path"), filepath.Join(home, "two")}
		if err := SetSourcePath(test.id, test.key, paths); err != nil {
			t.Fatal(err)
		}
		got := test.get()
		if len(got) != 2 || got[0] != paths[0] || got[1] != paths[1] {
			t.Fatalf("%s list torn: %v", test.id, got)
		}
	}
}

func TestSourcePathFieldResetPreservesOtherFields(t *testing.T) {
	home := isolatedPathSettings(t)
	if err := SetSourcePath("cursor", "ide", []string{filepath.Join(home, "ide")}); err != nil {
		t.Fatal(err)
	}
	if err := SetSourcePath("cursor", "cli", []string{filepath.Join(home, "cli")}); err != nil {
		t.Fatal(err)
	}
	if err := ResetSourcePathField("cursor", "ide"); err != nil {
		t.Fatal(err)
	}
	settings, err := ReadSourcePathSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Agents["cursor"].Paths["ide"] != nil || len(settings.Agents["cursor"].Paths["cli"]) != 1 {
		t.Fatalf("reset affected unrelated field: %+v", settings)
	}
}
