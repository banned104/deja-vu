package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vshulcz/deja-vu/internal/sources"
)

func cmdPaths(_ string, args []string) error { return runPaths(os.Stdout, args) }
func runPaths(out io.Writer, args []string) error {
	jsonOut := false
	var rest []string
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
		} else {
			rest = append(rest, arg)
		}
	}
	action := "list"
	if len(rest) > 0 {
		action = rest[0]
		rest = rest[1:]
	}
	if action == "--help" || action == "help" {
		fmt.Fprintln(out, "deja paths [list|show <agent>|set <agent> <field> <absolute-path>...|reset <agent>|enable <agent>|disable <agent>|check <agent>] [--json]")
		fmt.Fprintln(out, "Lists effective paths and their origin. Saves settings only; run deja index explicitly to update memory.")
		return nil
	}
	switch action {
	case "list", "show":
		if action == "list" && len(rest) != 0 || action == "show" && len(rest) != 1 {
			return fmt.Errorf("paths %s: invalid arguments; use paths --help", action)
		}
		infos, err := sources.SourcePathInfos()
		if err != nil {
			return err
		}
		if action == "show" {
			if len(sources.PathFields(rest[0])) == 0 {
				return fmt.Errorf("unknown agent %q", rest[0])
			}
			selected := infos[:0]
			for _, info := range infos {
				if info.Agent == rest[0] {
					selected = append(selected, info)
				}
			}
			infos = selected
		}
		if jsonOut {
			return json.NewEncoder(out).Encode(struct {
				Version int                      `json:"schema_version"`
				File    string                   `json:"config_file"`
				Fields  []sources.SourcePathInfo `json:"fields"`
			}{1, sources.SourcePathsFile(), infos})
		}
		fmt.Fprintln(out, "Agent history paths · "+sources.SourcePathsFile())
		for _, info := range infos {
			state := "enabled"
			if !info.Enabled {
				state = "disabled"
			}
			fmt.Fprintf(out, "%-14s %-10s %-11s %-11s %s\n  %s\n  %s · %s\n", info.Agent, info.Key, info.Kind, info.Origin, strings.Join(info.Paths, " | "), info.Label, state, info.Status)
		}
		return nil
	case "set":
		if len(rest) < 3 {
			return fmt.Errorf("paths set requires <agent> <field> <absolute-path>...")
		}
		if err := sources.SetSourcePath(rest[0], rest[1], rest[2:]); err != nil {
			return err
		}
	case "reset", "enable", "disable":
		if len(rest) != 1 {
			return fmt.Errorf("paths %s requires one agent", action)
		}
		var err error
		if action == "reset" {
			err = sources.ResetSourcePaths(rest[0])
		} else {
			err = sources.SetSourceEnabled(rest[0], action == "enable")
		}
		if err != nil {
			return err
		}
	case "check":
		if len(rest) != 1 {
			return fmt.Errorf("paths check requires one agent")
		}
		files, err := sources.CheckSource(rest[0])
		if err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(out).Encode(map[string]any{"schema_version": 1, "agent": rest[0], "files": files, "status": "available"})
		}
		fmt.Fprintf(out, "%s: %d source files found; no conversation content read, no index written\n", rest[0], files)
		return nil
	default:
		return fmt.Errorf("unknown paths action %q; use paths --help", action)
	}
	if jsonOut {
		return json.NewEncoder(out).Encode(map[string]any{"schema_version": 1, "saved": true, "index_updated": false})
	}
	fmt.Fprintln(out, "Saved. Index unchanged. Run deja index when you want to apply the new source selection.")
	return nil
}
