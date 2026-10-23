package sources

import (
	"os"
	"path/filepath"
	"testing"
)

// Direct launches have none of the environment the PI plugin sets. Use the
// application's ordinary default and honor both explicit root overrides.
func TestPiDesktopRootDefaultAndOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("DEJA_SOURCE_PATHS_FILE", filepath.Join(home, "source-paths.json"))
	t.Setenv("DEJA_STORES", "")
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "")
	t.Setenv("PI_DESKTOP_DATA_DIR", "")
	if got, want := PiDesktopRoot(), filepath.Join(home, ".pi-desktop"); got != want {
		t.Fatalf("direct launch root = %q, want %q", got, want)
	}
	custom := filepath.Join(home, "custom-data")
	t.Setenv("PI_DESKTOP_DATA_DIR", custom)
	if got := PiDesktopRoot(); got != custom {
		t.Fatalf("app root = %q", got)
	}
	explicit := filepath.Join(home, "selected-data")
	t.Setenv("DEJA_PI_DESKTOP_ROOT", explicit)
	if got := PiDesktopRoot(); got != explicit {
		t.Fatalf("deja root = %q", got)
	}
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "relative")
	if got := PiDesktopRoot(); got != "" {
		t.Fatalf("invalid explicit root fell back: %q", got)
	}
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "")
	t.Setenv("PI_DESKTOP_DATA_DIR", "relative")
	if got := PiDesktopRoot(); got != "" {
		t.Fatalf("invalid app root fell back: %q", got)
	}
	t.Setenv("DEJA_STORES", "notes")
	if got := PiDesktopRoot(); got != "" {
		t.Fatalf("excluded store resolved: %q", got)
	}
}

func TestPiDesktopLoadsDefaultStoreWithoutPluginEnvironment(t *testing.T) {
	root, _ := desktopFixture(t)
	t.Setenv("PI_DESKTOP_DATA_DIR", "")
	defaultRoot := filepath.Join(filepath.Dir(root), ".pi-desktop")
	if err := os.Rename(root, defaultRoot); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEJA_PI_DESKTOP_ROOT", "")
	files := PiDesktopSessionFiles()
	if len(files) != 1 {
		t.Fatalf("default discovery found %d files, want 1", len(files))
	}
	ss := LoadPiDesktop()
	if len(ss) != 1 || ss[0].Harness != "pi-desktop" || ss[0].ID != "desktop-demo" {
		t.Fatalf("direct launch lost Desktop history: %+v", ss)
	}
}
