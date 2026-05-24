# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build -o claudemap .        # build binary
go run . map                   # run without building
go test ./...                  # run all tests
go test ./analyze/...          # run a single package's tests
go test -run TestGlobMatch ./analyze/  # run a single test
go mod tidy                    # sync go.mod/go.sum after dep changes
```

## Architecture

The pipeline is: **Discover → Analyze → Render**, wired together by the cobra commands in `cmd/`.

**`model/model.go`** — all shared types. Read this first. Key types: `ClaudeFile` (one discovered file), `ContextAssembly` (the full result of discovery: eager + lazy files, composed blocks), `Finding` (a single issue). Everything else is built on these.

**`discover/`** — builds the `ContextAssembly` from disk.
- `discover.go`: orchestrates upward walk (workdir → root), user-level files (`~/.claude/`), managed policy, downward walk for lazy-dir files, and rules discovery. Deduplication via `seenPaths` prevents `~/.claude/CLAUDE.md` from appearing twice when the upward walk passes through home.
- `imports.go`: resolves `@path` references recursively, tracking depth (max 5) and circular refs via a seen-set.
- `platform.go`: returns the OS-specific managed policy path.

**`analyze/`** — stateless functions operating on `ContextAssembly`.
- `tokens.go`: `EstimateTokens` and `StripHTMLComments` (strips `<!-- -->` outside code blocks using placeholder substitution).
- `glob.go`: inline `**`-aware glob matcher (no external dep). Used for W02 dead-rule detection and simulate mode.
- `check.go`: all 9 issue detectors (E01–I03). Each detector iterates the assembly and appends `Finding` values.

**`render/`** — three output formats, all taking `(*ContextAssembly, []Finding)`.
- `terminal.go`: ANSI-colored tree. `NO_COLOR` / non-TTY detection strips colors.
- `json.go`: defines `Version` constant (single source of truth). Serializes to the SPEC §8 schema.
- `html.go`: single-function `HTML(...)` that templates the 4-panel dark-theme report (Overview / Context Tree / Composed Context / Findings) into a self-contained HTML file. The template is an inline string constant at the bottom of the file.

**`cmd/`** — cobra commands that wire the pipeline.
- `check.go`: runs discovery + checks, then branches on `--json` / `--quiet` / `--report html`. Calls `os.Exit` with code 0/1/2.
- `map.go`: runs discovery, renders terminal tree, handles `--compose` and `--simulate-open`. `collectSimulateFiles` computes which lazy files activate for the target path.

## Key conventions

- **Version** lives only in `render/json.go` as `const Version`.
- **Exit codes**: 0 = clean, 1 = info only, 2 = warning or error. Set via `os.Exit` in `cmd/check.go`.
- Load order: managed policy = 0, user files start at 1, ancestor files are collected then reversed (root-first) before assigning sequential order numbers.
- Lazy files never get a `LoadOrder`; they're identified by `LoadTiming == LoadLazyDir` or `LoadLazyRule`.
