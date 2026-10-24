package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type SourcePathField struct {
	Agent     string `json:"agent"`
	Key       string `json:"key"`
	Label     string `json:"label"`
	Kind      string `json:"kind"` // directory, directories, file
	Env       string `json:"env"`
	automatic func() []string
}

func onePath(fn func() string) func() []string {
	return func() []string {
		if p := fn(); p != "" {
			return []string{p}
		}
		return nil
	}
}

var pathCatalogOnce sync.Once
var pathCatalog []SourcePathField

func pathFieldTable() []SourcePathField {
	pathCatalogOnce.Do(func() {
		pathCatalog = []SourcePathField{
			{"pi-desktop", "data", "Application data (pi.sqlite + sessions)", "directory", "DEJA_PI_DESKTOP_ROOT", onePath(PiDesktopRoot)},
			{"claude", "sessions", "Projects/transcripts directory", "directory", "DEJA_CLAUDE_ROOT", ClaudeRoots},
			{"claude", "history", "Prompt history JSONL", "file", "DEJA_CLAUDE_HISTORY_FILE", onePath(ClaudeHistoryFile)},
			{"codex", "data", "Codex home (sessions + history)", "directory", "DEJA_CODEX_ROOT", onePath(CodexRoot)},
			{"opencode", "database", "Session SQLite database", "file", "DEJA_OPENCODE_DB", onePath(OpencodeDB)},
			{"opencode", "diffs", "Session diff directory", "directory", "DEJA_OPENCODE_DIFFS", onePath(OpencodeDiffDir)},
			{"aider", "file", "Chat history Markdown file", "file", "AIDER_CHAT_HISTORY_FILE", func() []string {
				return []string{EnvPath("AIDER_CHAT_HISTORY_FILE", filepath.Join(Home(), ".aider.chat.history.md"))}
			}},
			{"aider", "roots", "Project roots (one per line)", "directories", "DEJA_AIDER_ROOTS", func() []string { return filepath.SplitList(sourcePathOverride("DEJA_AIDER_ROOTS")) }},
			{"amp", "threads", "Thread JSON directory", "directory", "DEJA_AMP_ROOT", onePath(AmpRoot)},
			{"antigravity", "data", "Antigravity data root (brain)", "directory", "DEJA_ANTIGRAVITY_ROOT", AntigravityRoots},
			{"cline", "sessions", "Modern CLI/SDK session directory", "directory", "DEJA_CLINE_ROOT", onePath(ClineSessionsDir)},
			{"cline", "roots", "Legacy extension roots (each contains tasks)", "directories", "DEJA_CLINE_ROOTS", ClineLegacyRoots},
			{"cursor", "ide", "IDE User directory (globalStorage/workspaceStorage)", "directory", "DEJA_CURSOR_ROOT", onePath(CursorUserRoot)},
			{"cursor", "cli", "CLI config directory (projects/chats)", "directory", "DEJA_CURSOR_CLI_ROOT", onePath(CursorCLIRoot)},
			{"deepseek", "sessions", "Session root", "directory", "DEJA_DEEPSEEK_ROOT", onePath(DeepSeekRoot)},
			{"gemini", "data", "Gemini data root (tmp)", "directory", "DEJA_GEMINI_ROOT", onePath(GeminiRoot)},
			{"goose", "sessions", "Session directory", "directory", "DEJA_GOOSE_ROOT", onePath(GooseRoot)},
			{"goose", "database", "Session SQLite database", "file", "DEJA_GOOSE_DB", onePath(GooseDB)},
			{"grok", "data", "Grok data root (sessions)", "directory", "DEJA_GROK_ROOT", onePath(GrokRoot)},
			{"grok", "database", "Grok SQLite database", "file", "DEJA_GROK_DB", onePath(GrokDB)},
			{"hermes", "data", "Hermes home (state.db/profiles)", "directory", "DEJA_HERMES_HOME", onePath(HermesHome)},
			{"hermes", "profiles", "Profiles directory", "directory", "DEJA_HERMES_PROFILES_ROOT", onePath(HermesProfilesRoot)},
			{"hermes", "database", "Single SQLite database (overrides profiles)", "file", "DEJA_HERMES_DB", HermesDBs},
			{"kimi", "data", "Kimi home (sessions)", "directory", "DEJA_KIMI_ROOT", onePath(KimiRoot)},
			{"kimchi", "sessions", "Session directory", "directory", "DEJA_KIMCHI_ROOT", onePath(KimchiRoot)},
			{"omp", "sessions", "Session directory", "directory", "DEJA_OMP_ROOT", onePath(OmpRoot)},
			{"openclaw", "agents", "Agents directory", "directory", "DEJA_OPENCLAW_ROOT", onePath(OpenClawRoot)},
			{"continue", "data", "Continue home (sessions)", "directory", "DEJA_CONTINUE_ROOT", onePath(ContinueRoot)},
			{"crush", "data", "Crush data directory (projects.json)", "directory", "DEJA_CRUSH_ROOT", onePath(CrushDataHome)},
			{"pi", "sessions", "Session directory", "directory", "DEJA_PI_ROOT", onePath(PiRoot)},
			{"mcode", "sessions", "Session store (v2/sessions)", "directory", "DEJA_MCODE_ROOT", onePath(McodeRoot)},
			{"prime", "sessions", "Session directory", "directory", "DEJA_PRIME_ROOT", onePath(PrimeRoot)},
			{"qwen", "data", "Qwen home (projects)", "directory", "DEJA_QWEN_ROOT", onePath(QwenRoot)},
			{"cherrystudio", "roots", "Agent transcript roots (one per line)", "directories", "DEJA_CHERRYSTUDIO_ROOTS", CherryStudioAllRoots},
			{"senpi", "sessions", "Session directory", "directory", "DEJA_SENPI_ROOT", onePath(SenpiRoot)},
			{"gjc", "sessions", "Session directory", "directory", "DEJA_GJC_ROOT", onePath(GjcRoot)},
			{"commandcode", "projects", "Projects directory", "directory", "DEJA_COMMANDCODE_ROOT", onePath(CommandCodeRoot)},
			{"zcode", "projects", "Projects directory", "directory", "DEJA_ZCODE_ROOT", onePath(ZCodeRoot)},
			{"zcode", "database", "CLI SQLite database", "file", "DEJA_ZCODE_DB", onePath(ZCodeDB)},
			{"zcode", "legacy", "Legacy sessions root", "directory", "DEJA_ZCODE_LEGACY_ROOT", onePath(ZCodeLegacyRoot)},
			{"kiro", "sessions", "Session directory", "directory", "DEJA_KIRO_ROOT", onePath(KiroRoot)},
			{"kiro", "database", "CLI SQLite database", "file", "DEJA_KIRO_DB", onePath(KiroDB)},
			{"kilocode", "roots", "Extension task roots (one per line)", "directories", "DEJA_KILO_ROOTS", KiloRoots},
			{"kilocode", "database", "CLI SQLite database", "file", "DEJA_KILO_DB", onePath(KiloDB)},
			{"roo", "roots", "Extension task roots (one per line)", "directories", "DEJA_ROO_ROOTS", RooRoots},
			{"roo", "cli", "CLI global storage root", "directory", "DEJA_ROO_CLI_ROOT", onePath(RooCLIRoot)},
			{"zed", "data", "Zed data directory (threads)", "directory", "DEJA_ZED_ROOT", onePath(ZedRoot)},
			{"zed", "database", "Thread SQLite database", "file", "DEJA_ZED_DB", onePath(ZedDB)},
			{"devin", "database", "CLI session SQLite database", "file", "DEJA_DEVIN_DB", onePath(DevinSessionsDB)},
			{"codewhale", "sessions", "Session JSON directory", "directory", "DEJA_CODEWHALE_ROOT", onePath(CodeWhaleRoot)},
			{"junie", "data", "Junie home (sessions)", "directory", "DEJA_JUNIE_ROOT", onePath(JunieRoot)},
			{"jetbrains", "data", "IDE product configuration root", "directory", "DEJA_JETBRAINS_ROOT", onePath(JetBrainsRoot)},
			{"windsurf", "data", "Windsurf home (cascade)", "directory", "DEJA_WINDSURF_ROOT", onePath(WindsurfRoot)},
			{"codebuddy", "roots", "Project transcript roots (one per line)", "directories", "DEJA_CODEBUDDY_ROOTS", CodeBuddyRoots},
			{"reasonix", "data", "Reasonix state directory", "directory", "DEJA_REASONIX_ROOT", onePath(ReasonixRoot)},
			{"trae", "data", "TRAE CLI home (sessions)", "directory", "DEJA_TRAE_ROOT", onePath(TraeRoot)},
			{"muse", "sessions", "Session roots (one per line)", "directories", "DEJA_MUSE_ROOTS", MuseRoots},
			{"copilot", "sessions", "session-state directory", "directory", "DEJA_COPILOT_ROOT", onePath(CopilotRoot)},
			{"copilot-chat", "roots", "VS Code User directories (one per line)", "directories", "DEJA_COPILOT_CHAT_ROOTS", CopilotChatRoots},
		}
	})
	return pathCatalog
}
func PathFields(id string) []SourcePathField {
	var out []SourcePathField
	for _, field := range pathFieldTable() {
		if field.Agent == id {
			out = append(out, field)
		}
	}
	return out
}
func pathFieldByEnv(env string) (SourcePathField, bool) {
	for _, field := range pathFieldTable() {
		if field.Env == env {
			return field, true
		}
	}
	return SourcePathField{}, false
}

type SourcePathInfo struct {
	SourcePathField
	Paths   []string `json:"paths"`
	Origin  string   `json:"origin"`
	Enabled bool     `json:"enabled"`
	Read    bool     `json:"read"`
	Status  string   `json:"status"`
}

func SourcePathInfos() ([]SourcePathInfo, error) {
	settings, err := ReadSourcePathSettings()
	if err != nil {
		return nil, err
	}
	fields := pathFieldTable()
	out := make([]SourcePathInfo, 0, len(fields))
	for _, field := range fields {
		info := SourcePathInfo{SourcePathField: field, Enabled: !configuredSourceDisabled(field.Agent), Read: StoreRead(field.Agent), Origin: "automatic"}
		if v := os.Getenv(field.Env); v != "" {
			if field.Kind == "directories" {
				info.Paths = filepath.SplitList(v)
			} else {
				info.Paths = []string{v}
			}
			info.Origin = "environment"
		} else if paths := settings.Agents[field.Agent].Paths[field.Key]; len(paths) > 0 {
			info.Paths = paths
			info.Origin = "user"
		} else {
			info.Paths = field.automatic()
			if field.Agent == "pi-desktop" {
				if os.Getenv("PI_DESKTOP_DATA_DIR") != "" {
					info.Origin = "host env"
				} else if _, err := os.Stat(piDesktopPreferenceFile()); err == nil {
					info.Origin = "migration"
				}
			}
		}
		info.Status = checkSourcePaths(field, info.Paths)
		out = append(out, info)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
	return out, nil
}
func checkSourcePaths(field SourcePathField, paths []string) string {
	if len(paths) == 0 {
		return "not found"
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return "invalid: absolute path required"
		}
		stat, err := os.Stat(path)
		if err != nil {
			return "unavailable: " + err.Error()
		}
		if field.Kind == "file" && !stat.Mode().IsRegular() {
			return "invalid: expected file"
		}
		if field.Kind != "file" && !stat.IsDir() {
			return "invalid: expected directory"
		}
		if field.Agent == "pi-desktop" {
			db, err := os.Stat(filepath.Join(path, "pi.sqlite"))
			if err != nil || !db.Mode().IsRegular() {
				return "invalid: missing pi.sqlite database"
			}
			if sessions, err := os.Stat(filepath.Join(path, "sessions")); err == nil {
				if !sessions.IsDir() {
					return "invalid: sessions must be a directory"
				}
			} else if !os.IsNotExist(err) {
				return "unavailable: " + err.Error()
			}
		}
	}
	return "available"
}

// CheckSource only lists source files; it does not parse speech or build an index.
func CheckSource(id string) (int, error) {
	fields := PathFields(id)
	if len(fields) == 0 {
		return 0, fmt.Errorf("unknown agent %q", id)
	}
	infos, err := SourcePathInfos()
	if err != nil {
		return 0, err
	}
	for _, info := range infos {
		if info.Agent == id && info.Origin != "automatic" && info.Status != "available" {
			return 0, fmt.Errorf("%s: %s", info.Key, info.Status)
		}
	}
	for _, h := range StoresToRead() {
		if h.Name == id {
			return len(h.Files()), nil
		}
	}
	return 0, fmt.Errorf("source is disabled or excluded")
}

// SourcePathSettingsError validates only explicitly selected sources, without
// parsing history. A missing configured drive must not look like an empty store.
func SourcePathSettingsError() error {
	settings, err := ReadSourcePathSettings()
	if err != nil {
		return err
	}
	for id, entry := range settings.Agents {
		if StoreSilenced(id) || HarnessExcluded(id) {
			continue
		}
		for _, field := range PathFields(id) {
			paths := entry.Paths[field.Key]
			if raw := os.Getenv(field.Env); raw != "" {
				if field.Kind == "directories" {
					paths = filepath.SplitList(raw)
				} else {
					paths = []string{raw}
				}
			}
			if len(paths) > 0 {
				if status := checkSourcePaths(field, paths); status != "available" {
					return fmt.Errorf("%s.%s: %s; open Agent paths (P) or run deja paths to fix it", id, field.Key, status)
				}
			}
		}
	}
	return nil
}
