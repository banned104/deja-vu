# User-configurable Agent history paths

The terminal, CLI, hooks and MCP share one path settings file:
`~/.config/deja/source-paths.json`. `DEJA_SOURCE_PATHS_FILE` can explicitly name
another absolute file. This location deliberately does not follow a plugin's
isolated `XDG_CONFIG_HOME`, so changing a path in the terminal also changes what
the PI plugin reads, without reinstalling or repackaging.

## Terminal

Press **P** from the main screen, or **Ctrl+K → Agent history paths**. Every
supported Agent is listed, including ones that have no indexed history.

- `/`: search by Agent name or path field; Enter ends search.
- Enter / `e`: edit the selected field. Ctrl+U clears; paste is supported.
  Multi-directory fields accept one absolute directory per pasted line.
  Enter saves; Esc cancels. Fields say directory, directories or file.
- `c`: check the selected Agent's configured paths and enumerate source files.
  No conversation text is parsed, no index is written.
- `d`: restore automatic discovery for the selected field only, after `y`.
- Space: enable/disable **the whole Agent**, after explicit `y` confirmation.
- `i`: update the index after explicit confirmation. This reads all sources
  enabled in the current process. No rebuild is started merely by editing paths.
- Esc: close. The footer and help screen also expose the P shortcut.

Each row reports effective path, origin, saved enable state and current-read
state. `excluded` means an environment/privacy source restriction still applies;
saving Enabled cannot override that restriction.

## CLI

```powershell
deja paths list
deja paths show pi-desktop --json
deja paths set pi-desktop data D:\Pi-Desktop-Cache\data
deja paths check pi-desktop
deja paths disable pi-desktop
deja paths enable pi-desktop
deja paths reset pi-desktop
# Explicitly apply selected paths when ready:
deja index
```

Use `list` or `show` to find exact field keys; roots are not interchangeable:
Claude's projects directory differs from Codex's home, and a database field
expects a file. Sources with CLI/IDE/legacy stores have separately named fields.
A source can therefore legitimately show several rows.

`reset <agent>` restores all of that Agent's paths; the TUI restores one field.
`set` validates identifiers, path counts and absolute paths before atomic saving.
Missing drives may be saved for later use, but are marked unavailable, never
silently redirected, and an index update refuses to proceed until fixed or disabled.
Concurrent CLI/TUI updates serialize through a short-lived `.lock` file. If a
writer crashes, the error explains how to inspect and remove a stale lock safely.

## Resolution

Existing explicit `DEJA_*` source environment overrides win over saved settings.
Saved settings win over each Agent's own location/default discovery. The app
format parser is unchanged. For PI-Desktop specifically:

1. `DEJA_PI_DESKTOP_ROOT`;
2. saved `pi-desktop.data`;
3. `PI_DESKTOP_DATA_DIR`;
4. stable Electron profile `PI-Desktop/storage-location.json` → `roots.data`;
5. `~/.pi-desktop`.

A malformed migration pointer does not cause the old home directory to be read.
A data root with `pi.sqlite` but no `sessions/` yet is a valid empty installation.
On Linux, `DEJA_PI_DESKTOP_CONFIG_HOME` preserves the original host XDG base when
a plugin isolates its own XDG configuration. Migration backups are never merged.

The settings file contains paths/enable flags only, no credentials or history.
Example:

```json
{
  "version": 1,
  "agents": {
    "pi-desktop": {
      "enabled": true,
      "paths": { "data": ["D:\\Pi-Desktop-Cache\\data"] }
    }
  }
}
```

Source disable prevents scanning and filters recall of already-indexed rows; it
does not delete source history. Existing index retention/tombstone rules remain
in force. This feature never migrates, rewrites, deletes or scrubs Agent data.
