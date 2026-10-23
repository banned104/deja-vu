package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vshulcz/deja-vu/internal/atomicfile"
)

// SourcePathsFile is shared by the terminal, hooks, and MCP processes. Unlike
// policy/index roots it follows the user home even in a plugin's isolated XDG.
func SourcePathsFile() string {
	if p := os.Getenv("DEJA_SOURCE_PATHS_FILE"); p != "" {
		return p
	}
	if h := Home(); filepath.IsAbs(h) {
		return filepath.Join(h, ".config", "deja", "source-paths.json")
	}
	return ""
}

type SourcePathSetting struct {
	Enabled *bool               `json:"enabled,omitempty"`
	Paths   map[string][]string `json:"paths,omitempty"`
}
type SourcePathSettings struct {
	Version int                          `json:"version"`
	Agents  map[string]SourcePathSetting `json:"agents"`
}

var sourcePathCache struct {
	sync.Mutex
	path        string
	size, stamp int64
	identity    os.FileInfo
	value       SourcePathSettings
	err         error
}

func ReadSourcePathSettings() (SourcePathSettings, error) {
	value, err := readSourcePathSettings()
	return clonePathSettings(value), err
}

// Internal callers treat published snapshots as immutable.
func readSourcePathSettings() (SourcePathSettings, error) {
	path := SourcePathsFile()
	empty := SourcePathSettings{Version: 1, Agents: map[string]SourcePathSetting{}}
	if path == "" {
		return empty, nil
	}
	if !filepath.IsAbs(path) {
		return empty, fmt.Errorf("source paths config must be absolute: %s", path)
	}
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	if st.Size() > 1<<20 {
		return empty, fmt.Errorf("source paths config exceeds 1 MiB")
	}
	sourcePathCache.Lock()
	defer sourcePathCache.Unlock()
	if sourcePathCache.path == path && sourcePathCache.size == st.Size() && sourcePathCache.stamp == st.ModTime().UnixNano() && sourcePathCache.identity != nil && os.SameFile(sourcePathCache.identity, st) {
		return sourcePathCache.value, sourcePathCache.err
	}
	b, err := os.ReadFile(path)
	value := empty
	if err == nil {
		err = json.Unmarshal(b, &value)
	}
	if value.Agents == nil {
		value.Agents = map[string]SourcePathSetting{}
	}
	if err == nil {
		err = validatePathSettings(value)
	}
	if err != nil {
		err = fmt.Errorf("source paths config %s: %w", path, err)
	}
	sourcePathCache.path = path
	sourcePathCache.size = st.Size()
	sourcePathCache.stamp = st.ModTime().UnixNano()
	sourcePathCache.identity = st
	sourcePathCache.value = value
	sourcePathCache.err = err
	return value, err
}
func clonePathSettings(in SourcePathSettings) SourcePathSettings {
	out := SourcePathSettings{Version: in.Version, Agents: map[string]SourcePathSetting{}}
	for id, entry := range in.Agents {
		copy := SourcePathSetting{Paths: map[string][]string{}}
		if entry.Enabled != nil {
			v := *entry.Enabled
			copy.Enabled = &v
		}
		for key, paths := range entry.Paths {
			copy.Paths[key] = append([]string(nil), paths...)
		}
		out.Agents[id] = copy
	}
	return out
}
func validatePathSettings(value SourcePathSettings) error {
	if value.Version != 1 {
		return fmt.Errorf("unsupported version %d", value.Version)
	}
	for id, entry := range value.Agents {
		fields := PathFields(id)
		if len(fields) == 0 {
			return fmt.Errorf("unknown agent %q", id)
		}
		for key, paths := range entry.Paths {
			var field *SourcePathField
			for i := range fields {
				if fields[i].Key == key {
					field = &fields[i]
					break
				}
			}
			if field == nil {
				return fmt.Errorf("%s has no path field %q", id, key)
			}
			if len(paths) == 0 || (field.Kind != "directories" && len(paths) != 1) {
				return fmt.Errorf("%s: invalid path count for %s", id, key)
			}
			for _, path := range paths {
				if !filepath.IsAbs(path) {
					return fmt.Errorf("%s: %s requires absolute paths", id, key)
				}
			}
		}
	}
	return nil
}
func SaveSourcePathSettings(value SourcePathSettings) error {
	if err := validatePathSettings(value); err != nil {
		return err
	}
	path := SourcePathsFile()
	if !filepath.IsAbs(path) {
		return fmt.Errorf("cannot resolve absolute source paths config")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = atomicfile.Write(path, append(data, '\n'), 0600); err != nil {
		return err
	}
	sourcePathCache.Lock()
	sourcePathCache.path = ""
	sourcePathCache.Unlock()
	return nil
}

func SetSourcePath(id, key string, paths []string) error {
	unlock, err := lockSourcePathSettings()
	if err != nil {
		return err
	}
	defer unlock()
	value, err := ReadSourcePathSettings()
	if err != nil {
		return err
	}
	normalized := make([]string, len(paths))
	for i, p := range paths {
		normalized[i] = filepath.Clean(expandTilde(strings.TrimSpace(p)))
	}
	entry := value.Agents[id]
	if entry.Paths == nil {
		entry.Paths = map[string][]string{}
	}
	entry.Paths[key] = normalized
	value.Agents[id] = entry
	return SaveSourcePathSettings(value)
}
func ResetSourcePaths(id string) error {
	unlock, err := lockSourcePathSettings()
	if err != nil {
		return err
	}
	defer unlock()
	if len(PathFields(id)) == 0 {
		return fmt.Errorf("unknown agent %q", id)
	}
	value, err := ReadSourcePathSettings()
	if err != nil {
		return err
	}
	entry := value.Agents[id]
	entry.Paths = nil
	value.Agents[id] = entry
	return SaveSourcePathSettings(value)
}
func SetSourceEnabled(id string, enabled bool) error {
	unlock, err := lockSourcePathSettings()
	if err != nil {
		return err
	}
	defer unlock()
	if len(PathFields(id)) == 0 {
		return fmt.Errorf("unknown agent %q", id)
	}
	value, err := ReadSourcePathSettings()
	if err != nil {
		return err
	}
	entry := value.Agents[id]
	entry.Enabled = &enabled
	value.Agents[id] = entry
	return SaveSourcePathSettings(value)
}

// sourcePathOverride is called at existing path seams, never for credentials
// or parsing switches. It does not mutate the process environment.
func sourcePathOverride(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	field, ok := pathFieldByEnv(key)
	if !ok {
		return ""
	}
	value, err := readSourcePathSettings()
	if err != nil {
		diagFileError(SourcePathsFile(), err)
		// A malformed explicit configuration is not permission to scan defaults.
		return filepath.Join(filepath.Dir(SourcePathsFile()), ".invalid-source-path-config")
	}
	paths := value.Agents[field.Agent].Paths[field.Key]
	return strings.Join(paths, string(os.PathListSeparator))
}
func configuredSourceDisabled(id string) bool {
	value, err := readSourcePathSettings()
	if err != nil {
		diagFileError(SourcePathsFile(), err)
		return true
	}
	enabled := value.Agents[id].Enabled
	return enabled != nil && !*enabled
}

func lockSourcePathSettings() (func(), error) {
	path := SourcePathsFile()
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("source paths config must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		file, err := os.OpenFile(path+".lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			if err := file.Close(); err != nil {
				_ = os.Remove(path + ".lock")
				return nil, err
			}
			return func() { _ = os.Remove(path + ".lock") }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("Agent paths settings are being saved by another process; retry, or remove %s.lock only after confirming no writer is running", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// sourcePathList keeps JSON path arrays lossless, including path-list delimiter
// characters in directory names. Only explicit environment strings are split.
func sourcePathList(key string) []string {
	if raw := os.Getenv(key); raw != "" {
		return filepath.SplitList(raw)
	}
	field, ok := pathFieldByEnv(key)
	if !ok {
		return nil
	}
	value, err := readSourcePathSettings()
	if err != nil {
		diagFileError(SourcePathsFile(), err)
		return nil
	}
	return append([]string(nil), value.Agents[field.Agent].Paths[field.Key]...)
}

func ResetSourcePathField(id, key string) error {
	unlock, err := lockSourcePathSettings()
	if err != nil {
		return err
	}
	defer unlock()
	value, err := ReadSourcePathSettings()
	if err != nil {
		return err
	}
	valid := false
	for _, field := range PathFields(id) {
		if field.Key == key {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown Agent path %s.%s", id, key)
	}
	entry := value.Agents[id]
	delete(entry.Paths, key)
	value.Agents[id] = entry
	return SaveSourcePathSettings(value)
}
