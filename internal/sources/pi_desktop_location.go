package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func piDesktopPreferenceFile() string {
	home := Home()
	switch runtime.GOOS {
	case "windows":
		app := os.Getenv("APPDATA")
		if app == "" {
			app = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(app, "PI-Desktop", "storage-location.json")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "PI-Desktop", "storage-location.json")
	default:
		base := os.Getenv("XDG_CONFIG_HOME")
		if original := os.Getenv("DEJA_PI_DESKTOP_CONFIG_HOME"); original != "" {
			base = original
		}
		if !filepath.IsAbs(base) {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "PI-Desktop", "storage-location.json")
	}
}

// PI keeps this small pointer in Electron's stable profile, outside the moved
// history. Never merge backups or fall back to an old root after a bad pointer.
func piDesktopAutomaticRoot() string {
	if p := os.Getenv("PI_DESKTOP_DATA_DIR"); p != "" {
		return p
	}
	path := piDesktopPreferenceFile()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return filepath.Join(Home(), ".pi-desktop")
	}
	if err != nil {
		diagFileError(path, err)
		return ""
	}
	if len(b) > 64<<10 {
		diagFileError(path, fmt.Errorf("storage pointer exceeds 64 KiB"))
		return ""
	}
	var pointer struct {
		Version int `json:"version"`
		Roots   struct {
			Data string `json:"data"`
		} `json:"roots"`
	}
	if err := json.Unmarshal(b, &pointer); err != nil || pointer.Version != 1 || !filepath.IsAbs(pointer.Roots.Data) {
		diagFileError(path, fmt.Errorf("invalid PI-Desktop storage pointer"))
		return ""
	}
	return pointer.Roots.Data
}
