# claudemap

[![CI](https://github.com/alonw0/claudemap/actions/workflows/claudemap.yml/badge.svg)](https://github.com/alonw0/claudemap/actions/workflows/claudemap.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/alonw0/claudemap.svg)](https://pkg.go.dev/github.com/alonw0/claudemap)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Visibility into the CLAUDE.md context Claude Code actually loads.**

claudemap is a read-only CLI that walks your project directory the same way Claude Code does, assembles the full context, and shows you exactly which files load, in what order, how many tokens each consumes, and whether any structural or semantic issues exist — all without a network call.

## Install

**Homebrew (macOS / Linux):**
```bash
brew tap alonw0/claudemap
brew install claudemap
```

**Go install:**
```bash
go install github.com/alonw0/claudemap@latest
```

**Build from source:**
```bash
git clone https://github.com/alonw0/claudemap
cd claudemap
go build -o claudemap .
```

## Commands

### `claudemap map`

Renders the full context tree for the current directory: what loads eagerly at session start, what loads lazily based on which file you open, load order, line and token counts, and any structural issues.

```
claudemap map
claudemap map --workdir ~/myproject
claudemap map --simulate-open src/api/server.ts   # show what extra context loads for this file
claudemap map --compose                            # append full composed context after the tree
claudemap map --report html                        # open interactive HTML report
```

### `claudemap check`

Runs all issue detectors and reports findings. Use this in CI or as a pre-push check.

```
claudemap check
claudemap check --json                 # machine-readable output
claudemap check --quiet                # exit code only, no output
claudemap check --report html          # HTML report
claudemap check --report html --output report.html
```

**Exit codes:** `0` = clean · `1` = info only · `2` = warning or error

## Issue detectors

claudemap detects nine classes of issues that cause silent misbehavior in Claude Code:

| Code | Severity | Description |
|------|----------|-------------|
| E01 | error | Broken `@import` — file not found |
| E02 | error | Import chain depth > 5 (Claude silently drops it) |
| W01 | warning | File exceeds 200 lines |
| W02 | warning | Path-scoped rule matches no files in the tree |
| W03 | warning | Circular `@import` detected |
| W04 | warning | Circular symlink in `.claude/rules/` |
| I01 | info | File approaching 200-line limit (>=150 lines) |
| I02 | info | Lazy-dir file not duplicated in eager context (vanishes after `/compact`) |
| I03 | info | `claudeMdExcludes` pattern in settings matches no discovered files |

## HTML report

`claudemap map --report html` or `claudemap check --report html` generates a self-contained, single-file HTML report — no server, no external dependencies, works at `file://`.

**Five views:**

- **Overview** — health status, key metrics, token distribution bar, findings summary
- **Context Map** — proportional treemap of eager files by token weight; lazy files listed below
- **Context Tree** — full file tree with load order, scope, timing, line/token counts, import chains, and per-file finding indicators
- **Composed Context** — the full text Claude sees, in three modes:
  - *Blocks* — collapsible accordion, one section per file
  - *Eager* — all eager files concatenated in load order with sticky headers and line numbers
  - *All* — eager + lazy concatenated with section separators
  - *Copy All* copies clean text (no line numbers) to clipboard
- **Findings** — all findings with full detail, filterable by severity

Theme: light / system / dark, persisted to `localStorage`.

## Semantic analysis

### claudemap-analyze skill

The `claudemap-analyze` Claude Code skill extends structural analysis with semantic reasoning. It runs `claudemap check --json`, reads the full composed context, and asks Claude to identify rule conflicts, scope leakage, and ordering surprises — things structural checks cannot catch.

Install in one step:
```bash
claudemap install --skill           # install to ~/.claude/skills/ (global)
claudemap install --skill --local   # install to .claude/skills/ (project)
```

Then in any Claude Code session:
```
/skill claudemap-analyze
```

Claude identifies genuine conflicts (not surface similarities), states which rule wins for each ordering issue, and proposes minimal targeted fixes — asking for confirmation before changing anything.

### Session hooks

Automatically surface new issues at the start of your next session. The **stop hook** runs `claudemap suggest-updates` after each session: it baselines the current findings and writes a pending note if new errors or warnings have appeared. The **start hook** injects that note at the top of the next session so Claude proactively offers to fix them.

Set up everything — skill and hooks — in one command:
```bash
claudemap install                   # skill globally + hooks in current project
claudemap install --skill           # skill only (~/.claude/skills/)
claudemap install --skill --local   # skill in ./.claude/skills/
claudemap install --hooks           # hooks only (current project)
claudemap install --hooks --global  # hooks in ~/.claude/settings.json
```

See [`docs/hooks.md`](docs/hooks.md) for manual setup.

## GitHub Action / CI

claudemap ships as a reusable GitHub Action. Add it to any repo to enforce CLAUDE.md hygiene on every pull request.

### Structural check

```yaml
# .github/workflows/claudemap.yml
name: claudemap
on:
  pull_request:
    paths: ['**CLAUDE.md', '**CLAUDE.local.md', '.claude/rules/**']

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - uses: alonw0/claudemap@main
        with:
          fail-on: 'warning'   # or 'error' to allow warnings through
```

**Inputs:**

| Input | Default | Description |
|-------|---------|-------------|
| `workdir` | `.` | Directory to run `claudemap check` in |
| `fail-on` | `warning` | Minimum severity to fail: `error`, `warning`, or `never` |
| `version` | `latest` | claudemap version to install |

**What you get:**
- Inline PR annotations on the exact lines with issues
- A job summary table with all findings
- Exit code 0 (clean) or 2 (findings above threshold)

### Structural + semantic (full CI)

A second parallel job pipes the composed context to Claude for semantic analysis — catching rule conflicts, scope leakage, and ordering surprises that structural checks miss. The semantic job is advisory: it posts findings to the job summary but never blocks a merge.

Requires an `ANTHROPIC_API_KEY` repo secret.

```yaml
# .github/workflows/claudemap.yml
name: claudemap
on:
  pull_request:
    paths: ['**CLAUDE.md', '**CLAUDE.local.md', '.claude/rules/**']

jobs:
  structural:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - uses: alonw0/claudemap@main
        with:
          fail-on: 'warning'

  semantic:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
      - name: Install claudemap and Claude Code CLI
        run: |
          go install github.com/alonw0/claudemap@latest
          npm install -g @anthropic-ai/claude-code --quiet
      - name: Analyze for rule conflicts
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          claudemap check --json > /tmp/cm.json
          python3 - <<'EOF'
          import json, subprocess, sys, os

          with open('/tmp/cm.json') as f:
              data = json.load(f)

          blocks = data["assembly"]["composed_blocks"]
          findings = data["findings"]

          prompt = (
              "Analyze this Claude Code project's assembled CLAUDE.md configuration.\n\n"
              "Load order matters: higher load-order numbers win when rules conflict.\n\n"
              "Flag only genuine issues:\n"
              "- CONFLICT: two rules giving opposite instructions for the same situation\n"
              "- SCOPE: personal preference that belongs in ~/.claude/CLAUDE.md\n"
              "- ORDERING: a rule silently overridden by a later-loading file\n"
              "- OVERSPEC: a broad-file rule that belongs in a scoped .claude/rules/ file\n\n"
              "If no genuine issues exist, say so clearly. Do not manufacture findings.\n\n"
              "<context>\n" + json.dumps(blocks, indent=2) + "\n</context>\n\n"
              "<structural_findings>\n" + json.dumps(findings, indent=2) + "\n</structural_findings>"
          )

          result = subprocess.run(["claude", "-p", prompt], capture_output=True, text=True)
          output = result.stdout.strip() or result.stderr.strip()
          print(output)

          summary = os.environ.get("GITHUB_STEP_SUMMARY", "")
          if summary:
              with open(summary, "a") as f:
                  f.write("## Semantic analysis\n\n")
                  f.write(output + "\n")
          EOF
```

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

`composed_blocks` contains the full text of each eager file in load order — the exact context Claude receives at session start.
