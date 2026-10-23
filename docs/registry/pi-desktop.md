# PI-Desktop

**Last verified:** 2026-10-22 (source and synthetic fixture; no private store)

Verified against local PI-Desktop fc8d396ce (transcript schema 1).

## Discovery and consent

Root precedence: `DEJA_PI_DESKTOP_ROOT`, shared user Agent paths settings,
`PI_DESKTOP_DATA_DIR`, the host's `storage-location.json` migration pointer,
then `~/.pi-desktop`. An explicit missing root or malformed pointer does not
fall back to stale history. Only one active installation is selected.
Open the terminal's **P** Agent paths page, or run `deja paths show pi-desktop`
and `deja paths set pi-desktop data <absolute-data-dir>`. See
[`../SOURCE-PATHS.md`](../SOURCE-PATHS.md). `DEJA_STORES=pi-desktop,notes` and an
independent `DEJA_INDEX_DIR` isolate integration tests. sqlite3 must be on PATH.

Reads flat `sessions/<id>.jsonl` and, read-only, the corresponding `sessions`
and `projects` rows in `pi.sqlite`. Database and WAL stat changes invalidate
transcript metadata. Revisions, inflight checkpoints, soft-deleted sessions,
orphan transcripts and native pi sessions are not separate sources here.

## Records

The header is `{type:session,schema:1,sessionId,createdAt}`. Message records
carry `id`, `role`, `createdAt` and a canonical `blocks` array. Duplicate IDs
are last-write-wins. Text blocks become speech; thinking and attachments are
not indexed. Tool rows contain `tool_call` blocks with name/args/result/status;
commands, file paths and tool output retain their roles and explicit exit codes.
Compaction records retain their summary. Nested delegate rows are skipped.

Only a torn last line is ignored. Unknown schema, identity mismatch and malformed
complete records are errors rather than successful empty histories. Files are
re-read whole on change: regeneration and duplicate IDs make append parsing unsafe.

## Integration and limitations

The independent local PI plugin declares the MCP server and maps Desktop's
extension lifecycle to deja hooks. This source does not install anything into a
running application. The parser registry permits compaction capture by harness
`pi-desktop` and the native session ID; project identity comes from SQLite.

No direct index access, cloud service, native resume, source deletion/writeback,
or reconstructed removed code is provided. Line-anchored Edit instructions do
not prove removed bytes, so this reader does not manufacture edit spans. Memory
is historical evidence, not new instructions. Test with synthetic data before
explicitly authorizing an installation's private history.
