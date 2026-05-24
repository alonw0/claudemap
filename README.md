# claudemap

A zero-network, read-only CLI that makes Claude Code's assembled CLAUDE.md context visible. See exactly what files load, in what order, how many tokens they consume, and whether any structural or semantic issues exist.

## Install

```bash
go install github.com/alonw0/claudemap@latest
```

Or build from source:
```bash
git clone https://github.com/alonw0/claudemap
cd claudemap
go build -o claudemap .
```

## Commands

### `claudemap map`

Shows the full context tree for the current directory — what Claude loads eagerly at session start, what loads lazily, and any structural issues.

```
claudemap map
claudemap map --workdir ~/myproject
claudemap map --simulate-open src/api/server.ts   # show what extra context loads for this file
claudemap map --compose                            # append full composed context after the tree
claudemap map --report html                        # open interactive HTML report
```

### `claudemap check`

Runs all issue detectors and reports findings.

```
claudemap check
claudemap check --json                 # machine-readable output
claudemap check --quiet                # exit code only, no output
claudemap check --report html          # HTML report
claudemap check --report html --output report.html
```

**Exit codes:** `0` = clean · `1` = info only · `2` = warning or error

## Issue detectors

| Code | Severity | Description |
|------|----------|-------------|
| E01 | error | Broken `@import` — file not found |
| E02 | error | Import chain depth > 5 (Claude silently drops it) |
| W01 | warning | File exceeds 200 lines |
| W02 | warning | Path-scoped rule matches no files in the tree |
| W03 | warning | Circular `@import` detected |
| W04 | warning | Circular symlink in `.claude/rules/` |
| I01 | info | File approaching 200-line limit (≥150 lines) |
| I02 | info | Lazy-dir file not duplicated in eager context (vanishes after `/compact`) |
| I03 | info | `claudeMdExcludes` pattern in settings matches no discovered files |

## HTML report

`claudemap map --report html` or `claudemap check --report html` generates a self-contained HTML file (no external dependencies, works at `file://`).

**Views:**
- **Overview** — health status, key metrics, token distribution bar, findings summary
- **Context Map** — proportional treemap of eager files by token weight; lazy files as pills below
- **Context Tree** — full file tree with load order, scope, timing, line/token counts, import chains, and per-file finding indicators
- **Composed Context** — three modes:
  - **Blocks** — collapsible accordion per file
  - **Eager** — all eager files concatenated in load order with sticky file headers and line numbers
  - **All** — eager + lazy concatenated, separated by section headers
  - **Copy all** button copies clean text (without line numbers) to clipboard
- **Findings** — all findings with full detail, filterable by severity

**Theme:** light / system / dark toggle, persisted to `localStorage`.

## Phase 2: Semantic analysis

### Analysis skill

The `.claude/skills/claudemap-analyze.md` skill brings semantic analysis inside Claude Code. It runs `claudemap check --json`, reads the full composed context, and reasons about rule conflicts, scope leakage, and ordering surprises.

Install:
```bash
cp .claude/skills/claudemap-analyze.md ~/.claude/skills/
```

Use in any Claude Code session:
```
/skill claudemap-analyze
```

Claude will identify genuine conflicts (not surface similarities), state which rule wins for each ordering issue, and propose minimal targeted fixes — asking for confirmation before making any change.

### Session hooks (opt-in)

Automatically surface new issues at the start of your next session. See [`docs/hooks.md`](docs/hooks.md) for setup.

The **stop hook** (`scripts/claudemap-suggest-updates`) baselines findings after each session and writes a pending message if new ERR/WARN issues appear. The **start hook** injects the pending message at the top of the next session.

## JSON output schema

`claudemap check --json` produces:

```json
{
  "claudemap_version": "0.1.0",
  "workdir": "/path/to/project",
  "timestamp": "2026-05-24T12:00:00Z",
  "assembly": {
    "eager_token_total": 1234,
    "eager_line_total": 89,
    "eager_files": [...],
    "lazy_files": [...],
    "composed_blocks": [
      {
        "source_file": "/path/to/CLAUDE.md",
        "scope": "project_root",
        "load_order": 3,
        "content": "...",
        "tokens": 412
      }
    ]
  },
  "findings": [...],
  "summary": { "total_findings": 2, "errors": 1, "warnings": 1, "info": 0 }
}
```

`composed_blocks` contains the full text of each eager file in load order — the complete context Claude receives at session start.
