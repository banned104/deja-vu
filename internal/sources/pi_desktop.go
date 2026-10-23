package sources

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vshulcz/deja-vu/internal/model"
)

// PiDesktopRoot follows explicit overrides, saved user settings, the host's
// migration pointer, then ~/.pi-desktop. Only one active root is read.
func PiDesktopRoot() string {
	if StoreSilenced("pi-desktop") {
		return ""
	}
	p := strings.TrimSpace(sourcePathOverride("DEJA_PI_DESKTOP_ROOT"))
	if p == "" {
		p = piDesktopAutomaticRoot()
	}
	if p == "" {
		return ""
	}
	p = expandTilde(p)
	if !filepath.IsAbs(p) {
		return ""
	}
	return filepath.Clean(p)
}

func piDesktopFile(p string) bool {
	root := PiDesktopRoot()
	if root == "" || filepath.Clean(filepath.Dir(p)) != filepath.Join(root, "sessions") {
		return false
	}
	name := filepath.Base(p)
	return strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".revisions.jsonl")
}

func PiDesktopSessionFiles() []string {
	root := PiDesktopRoot()
	if root == "" {
		return nil
	}
	return walkFiles(filepath.Join(root, "sessions"), piDesktopFile)
}

func LoadPiDesktop() []model.Session {
	return parseFiles(PiDesktopSessionFiles(), ParsePiDesktopFile)
}

// Metadata stays outside the transcript in Desktop schema 24. Read only these
// two tables; never inspect provider configuration, kv, or secret references.
type piDesktopMeta struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	CWD     string `json:"cwd"`
	Deleted bool   `json:"deleted"`
}

func piDesktopMetadata(path string) (*piDesktopMeta, error) {
	db := filepath.Join(filepath.Dir(filepath.Dir(path)), "pi.sqlite")
	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	query := "SELECT json_object('id',s.id,'title',s.title,'cwd',COALESCE(p.path,''),'deleted',s.deleted_at IS NOT NULL) FROM sessions s LEFT JOIN projects p ON p.id=s.project_id WHERE s.id='" + strings.ReplaceAll(id, "'", "''") + "';"
	out, err := sqliteOutput(db, query)
	if err != nil {
		return nil, fmt.Errorf("read PI-Desktop session metadata: %w", err)
	}
	// sqlite emits 0/1 for boolean expressions.
	rows, err := sqliteObjects[struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		CWD     string `json:"cwd"`
		Deleted int    `json:"deleted"`
	}](out)
	if err != nil {
		return nil, fmt.Errorf("decode PI-Desktop metadata: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil // orphan/purged transcript: not a live session
	}
	r := rows[0]
	return &piDesktopMeta{r.ID, r.Title, r.CWD, r.Deleted != 0}, nil
}

// ParsePiDesktopFile always re-reads the session. Message IDs are last-write-
// wins, and regenerate can rewrite the file. It is NOT an append-only pi log.
func ParsePiDesktopFile(path string) ([]model.Session, error) {
	if !piDesktopFile(path) {
		return nil, fmt.Errorf("PI-Desktop transcript outside the explicitly selected root")
	}
	root, err := filepath.EvalSymlinks(PiDesktopRoot())
	if err != nil {
		return nil, err
	}
	for _, candidate := range []string{path, filepath.Join(PiDesktopRoot(), "pi.sqlite")} {
		real, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, real)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("PI-Desktop source symlink escapes selected root")
		}
	}
	meta, err := piDesktopMetadata(path)
	if err != nil || meta == nil || meta.Deleted {
		return nil, err
	}
	return parsePiDesktopTranscript(path, *meta)
}

func piDesktopSidecar(path string) (int64, int64) {
	db := filepath.Join(filepath.Dir(filepath.Dir(path)), "pi.sqlite")
	return sidecarStat(db, db+"-wal")
}

// PiDesktopWorkspace resolves the project using the same ownership metadata as
// indexing. It does not guess from a project basename or transcript folder.
func PiDesktopWorkspace(path string) string {
	m, err := piDesktopMetadata(path)
	if err != nil || m == nil || m.Deleted {
		return ""
	}
	return m.CWD
}

type piDesktopLine struct {
	Type      string           `json:"type"`
	Schema    int              `json:"schema"`
	SessionID string           `json:"sessionId"`
	ID        string           `json:"id"`
	Role      string           `json:"role"`
	CreatedAt string           `json:"createdAt"`
	Summary   string           `json:"summary"`
	IsError   bool             `json:"isError"`
	Meta      map[string]any   `json:"meta"`
	Blocks    []map[string]any `json:"blocks"`
}

func parsePiDesktopTranscript(path string, meta piDesktopMeta) ([]model.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil {
		return nil, err
	} else if info.Size() > 64<<20 {
		return nil, fmt.Errorf("PI-Desktop transcript exceeds 64 MiB reader budget")
	}
	s := model.Session{Harness: "pi-desktop", ID: meta.ID, Title: meta.Title,
		Project: cwdProjectName(meta.CWD), Path: path}
	if s.Project == "" {
		s.Project = "pi-desktop-unbound"
	}
	var lines []piDesktopLine
	positions := map[string]int{}
	reader := bufio.NewReaderSize(f, 64<<10)
	header := false
	for {
		b, readErr := reader.ReadBytes('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		// Only a torn final line is ignored. Corruption in a committed line
		// must not turn a previously indexed history into an empty success.
		if len(b) > 0 && b[len(b)-1] == '\n' {
			var l piDesktopLine
			if err := json.Unmarshal(b, &l); err != nil {
				return nil, fmt.Errorf("PI-Desktop transcript: %w", err)
			}
			if !header {
				if l.Type != "session" || l.Schema != 1 || l.SessionID != meta.ID {
					return nil, fmt.Errorf("unsupported PI-Desktop schema or mismatched session identity")
				}
				header = true
				s.Touch(parseTimeAny(l.CreatedAt))
			} else if l.Type == "message" || l.Type == "compaction" {
				if l.ID == "" {
					return nil, fmt.Errorf("PI-Desktop record missing id")
				}
				key := l.Type + ":" + l.ID
				if i, ok := positions[key]; ok {
					lines[i] = l
				} else {
					positions[key] = len(lines)
					lines = append(lines, l)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	if !header {
		return nil, fmt.Errorf("PI-Desktop transcript missing complete header")
	}
	r := newPiReader(&s, false)
	r.cwd = meta.CWD
	for _, l := range lines {
		at := parseTimeAny(l.CreatedAt)
		s.Touch(at)
		if l.Type == "compaction" {
			if strings.TrimSpace(l.Summary) != "" {
				r.add(RoleSummary, l.Summary, at)
			}
			continue
		}
		// Nested delegate rows are already represented by their Task result.
		if str(l.Meta["parentToolCallId"]) != "" {
			continue
		}
		switch l.Role {
		case "user", "assistant":
			var text []string
			for _, b := range l.Blocks {
				if b["type"] == "text" {
					if t := str(b["text"]); t != "" {
						text = append(text, t)
					}
				}
			}
			if t := strings.Join(text, "\n"); strings.TrimSpace(t) != "" {
				r.add(l.Role, t, at)
			}
		case "tool":
			for _, b := range l.Blocks {
				if b["type"] != "tool_call" {
					continue
				}
				status := str(b["status"])
				if status == "running" || status == "denied" {
					continue
				}
				name := strings.ToLower(str(b["name"]))
				args, _ := b["args"].(map[string]any)
				args = piDesktopArgs(args)
				failed := l.IsError || b["isError"] == true || status == "error"
				id := str(b["callId"])
				// Desktop line-anchored edits do not retain removed bytes. Keep
				// paths/commands, never invent edit spans from PUT instructions.
				r.call(id, name, args, false, at)
				text := str(b["text"])
				result := b["result"]
				if text == "" {
					text = piDesktopResultText(result)
				}
				if text != "" && IndexToolOutput() {
					r.add(RoleToolOutput, text, at)
				}
				details, _ := result.(map[string]any)
				if nested, ok := details["details"].(map[string]any); ok {
					details = nested
				}
				r.toolResult(map[string]any{"toolCallId": id, "toolName": name, "isError": failed,
					"details": details, "content": text}, at)
			}
		}
	}
	r.finish()
	if len(s.Messages) == 0 {
		return nil, nil
	}
	return []model.Session{s}, nil
}

func piDesktopArgs(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	if out["path"] == nil {
		out["path"] = out["file_path"]
	}
	return out
}

func piDesktopResultText(v any) string {
	if text := contentText(v); text != "" {
		return text
	}
	m, _ := v.(map[string]any)
	var parts []string
	for _, key := range []string{"stdout", "stderr"} {
		if text := str(m[key]); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n")
	}
	for _, key := range []string{"text", "content"} {
		if text := contentText(m[key]); text != "" {
			return text
		}
	}
	return ""
}
