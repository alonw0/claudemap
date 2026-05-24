# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build -o claudemap .                          # build binary
go run . map                                     # run without building
go run . check --json | jq .                     # JSON output
go run . check --report html                     # HTML report
go test ./...                                    # run all tests
go test ./analyze/...                            # run a single package's tests
go test -run TestGlobMatch ./analyze/            # run a single test
go mod tidy                                      # sync go.mod/go.sum after dep changes
```

## Architecture

The pipeline is: **Discover → Analyze → Render**, wired together by the cobra commands in `cmd/`.

**`model/model.go`** — all shared types. Read this first. Key types: `ClaudeFile` (one discovered file), `ContextAssembly` (the full result of discovery: eager + lazy files, composed blocks), `Finding` (a single issue). Everything else is built on these.

**`discover/`** — builds the `ContextAssembly` from disk.
- `discover.go`: orchestrates upward walk (workdir → root), user-level files (`~/.claude/`), managed policy, downward walk for lazy-dir files, and rules discovery. Deduplication via `seenPaths` prevents `~/.claude/CLAUDE.md` from appearing twice. Upward walk collects per-directory groups, reverses groups (root-first), but preserves within-group order (CLAUDE.md < CLAUDE.local.md < .claude/CLAUDE.md < .claude/CLAUDE.local.md). `walkSubdirs` skips `workdir/.claude/` to avoid double-counting eager files as lazy.
- `imports.go`: resolves `@path` references recursively, tracking depth (max 5) and circular refs via a seen-set.
- `platform.go`: returns the OS-specific managed policy path.

**`analyze/`** — stateless functions operating on `ContextAssembly`.
- `tokens.go`: `EstimateTokens` and `StripHTMLComments` (strips `<!-- -->` outside code blocks using placeholder substitution).
- `glob.go`: inline `**`-aware glob matcher (no external dep). Used for W02 dead-rule detection and simulate mode.
- `check.go`: all 9 issue detectors (E01–I03). I03 matches `claudeMdExcludes` patterns against absolute paths and checks both `~/.claude/settings.json` and project-level settings files.

**`render/`** — three output formats, all taking `(*ContextAssembly, []Finding)`.
- `terminal.go`: ANSI-colored tree. `NO_COLOR` / non-TTY detection strips colors.
- `json.go`: defines `Version` constant (single source of truth). Serializes to the JSON schema including `composed_blocks` (the full text of each eager file in load order).
- `html.go`: single-function `HTML(...)` that templates the 5-view report into a self-contained HTML file. The template is an inline string constant at the bottom of the file. Views: Overview, Context Map, Context Tree, Composed Context (Blocks/Eager/All modes with line numbers and Copy All), Findings. Supports light/dark/system themes.

**`cmd/`** — cobra commands that wire the pipeline.
- `check.go`: runs discovery + checks, then branches on `--json` / `--quiet` / `--report html`. Calls `os.Exit` with code 0/1/2.
- `map.go`: runs discovery, renders terminal tree, handles `--compose` and `--simulate-open`. `collectSimulateFiles` computes which lazy files activate for the target path.

**`.claude/skills/claudemap-analyze.md`** — Claude Code skill for semantic analysis. Install to `~/.claude/skills/` and invoke with `/skill claudemap-analyze`. Runs `claudemap check --json`, reads `composed_blocks`, and analyzes for rule conflicts, scope leakage, and ordering surprises.

**`scripts/claudemap-suggest-updates`** — stop hook script. Diffs findings against `.claude/claudemap-baseline.json`; writes `.claude/claudemap-pending.md` when new ERR/WARN issues appear. See `docs/hooks.md` for full setup.

## Key conventions

- **Version** lives only in `render/json.go` as `const Version`.
- **Exit codes**: 0 = clean, 1 = info only, 2 = warning or error. Set via `os.Exit` in `cmd/check.go`.
- Load order: managed policy = 0, user files start at 1, ancestor directory groups reversed (root-first), then sequential within each group.
- Lazy files never get a `LoadOrder`; they're identified by `LoadTiming == LoadLazyDir` or `LoadLazyRule`.
- HTML template is one large inline string in `html.go` — all CSS, JS, and markup in a single constant. Edit it directly; there is no separate template file.
- `claudeMdExcludes` in settings.json matches absolute file paths (not relative), per Claude Code docs.
