# Product Requirements Document
# claudemap v2 — CLAUDE.md Context Analyzer

**Version:** 2.0  
**Status:** Draft  
**Last Updated:** 2026-05-24

---

## 1. Overview

### 1.1 Problem Statement

Claude Code assembles its context from multiple CLAUDE.md files dynamically, with the resulting composition depending on the working directory from which it is launched. Most users — including experienced engineers — don't understand this propagation model. They write rules that silently conflict, create files in the wrong scope, over-stuff the root CLAUDE.md, or write instructions that disappear after `/compact` with no warning.

There is no existing tool that makes the assembled context visible. `claudemap` fills that gap.

### 1.2 What This Tool Does (and Doesn't Do)

**Does:**
- Discover every CLAUDE.md-family file relevant to a given working directory
- Show them in a tree with load order, scope, and eager vs. lazy classification
- Resolve `@import` chains and surface broken or deep ones
- Detect dead path-scoped rules (globs that match nothing)
- Estimate token cost per file using a dependency-free heuristic
- Output the fully composed eager context as Claude will receive it

**Does not:**
- Detect semantic conflicts between rules (that's the job of a future Claude Code skill/hook — see `FUTURE.md`)
- Apply any fixes automatically
- Connect to the internet or call any API
- Require any API key

### 1.3 Design Principles

- **Zero dependencies** — single binary, works offline, runs in CI without setup
- **Read-only** — never writes anything, never modifies files
- **Conservative** — when in doubt about whether something is a real issue, emit INFO not WARNING
- **Fast** — complete in under 3 seconds on any realistic project

---

## 2. Target Users

**Individual developers** using Claude Code on a project with more than one CLAUDE.md file, who encounter unexpected Claude behavior and want to understand why.

**Platform / DevEx teams** deploying Claude Code across an organization, who need to audit and standardize CLAUDE.md hierarchies across multiple repositories or a monorepo.

---

## 3. User Stories

| ID | As a... | I want to... | So that... |
|----|---------|-------------|------------|
| U1 | Developer | See every CLAUDE.md file Claude will load from my current directory | I understand what context Claude actually has |
| U2 | Developer | See files in load order, clearly labeled eager vs. lazy | I understand which rules take precedence |
| U3 | Developer | See estimated token cost per file and total | I can make informed decisions about what to trim |
| U4 | Developer | Simulate what context Claude would have if it opened a specific file | I can debug why a rule in a subdirectory isn't firing |
| U5 | Developer | See the fully composed eager context as one document | I can read exactly what Claude reads |
| U6 | Developer | Be warned about broken `@imports` | I catch silent failures before they cost me a session |
| U7 | Developer | See which path-scoped rules are dead (glob matches nothing) | I can remove noise from my configuration |
| U8 | Developer | See rules that only exist in lazy files and won't survive `/compact` | I know which rules disappear mid-session |
| U9 | DevEx team | Run claudemap in CI and get a non-zero exit code on errors | Configuration regressions are caught before merge |
| U10 | DevEx team | Get JSON output | I can pipe findings into other tools or dashboards |

---

## 4. Commands

```
claudemap map              Show the context tree for the current directory
claudemap map --workdir    Run against a different directory
claudemap map --compose    Also print the full composed eager context to stdout
claudemap map --simulate-open <file>   Show what additional context loads for that file

claudemap check            Run all issue detectors, print findings
claudemap check --json     Machine-readable output
claudemap check --quiet    Exit code only (for CI)

claudemap version          Print version
```

No `fix` command in v1. Fixes are out of scope until the skill/hook layer exists (see `FUTURE.md`).

---

## 5. Feature Requirements

### 5.1 Discovery

The tool must find:

- All `CLAUDE.md`, `CLAUDE.local.md`, `.claude/CLAUDE.md` files by walking **up** the directory tree from workdir to filesystem root
- All `CLAUDE.md` / `CLAUDE.local.md` files in **subdirectories** of workdir (classified as lazy)
- `.claude/rules/*.md` files (recursive), parsed for YAML frontmatter `paths:` globs
- `~/.claude/CLAUDE.md` and `~/.claude/rules/` (user-level)
- Platform-specific managed policy file (macOS / Linux / Windows paths)
- Symlinks in `.claude/rules/` — followed, with circular symlink detection

### 5.2 Classification

Each discovered file must be labeled with:

- **Scope**: managed policy / user / project-root / project-local / subdirectory / rule
- **Load timing**: eager (loads at session start) or lazy (loads when Claude opens a matching file)
- **Load order number** within the eager sequence

### 5.3 Import Resolution

- Parse all `@path/to/file` references in every discovered file
- Resolve relative paths correctly (relative to the containing file, not workdir)
- Track depth (max 5 hops per spec)
- Flag: missing file (ERROR), depth exceeded (ERROR), circular reference (WARNING)
- Display the import graph inline in the tree

### 5.4 Token Estimation

Dependency-free heuristic. No external libraries, no API calls.

```
1. Strip HTML comments (<!-- ... --> outside code blocks)
2. Identify fenced code blocks (``` ... ```)
3. prose_tokens  = prose_chars  / 4.0
4. code_tokens   = code_block_chars / 3.5
5. total = ceil(prose_tokens + code_tokens)
```

Label all estimates as `~N tokens`. Line count is displayed alongside as the primary health indicator (threshold: 200 lines per official docs).

### 5.5 Issue Detection

| ID | Name | Severity | Description |
|----|------|----------|-------------|
| E01 | broken-import | ERROR | `@import` points to a file that doesn't exist |
| E02 | import-depth | ERROR | `@import` chain exceeds 5 hops (Claude silently drops it) |
| W01 | size-violation | WARNING | File exceeds 200 lines |
| W02 | dead-rule | WARNING | Path-scoped rule's glob matches zero files in workdir tree |
| W03 | circular-import | WARNING | `@import` cycle detected |
| W04 | circular-symlink | WARNING | Symlink cycle detected in `.claude/rules/` |
| I01 | size-approaching | INFO | File is 150–200 lines |
| I02 | post-compaction-risk | INFO | Rule exists only in a lazy subdirectory file; will not survive `/compact` until Claude opens a file there |
| I03 | dead-exclude | INFO | `claudeMdExcludes` pattern in settings matches no actual files |

No scope-mismatch heuristic, no missing-coverage detection, no semantic conflict detection. Those belong in the skill/hook layer.

### 5.6 Simulate Mode

`claudemap map --simulate-open <path>` shows:

1. Normal eager context (same as regular map)
2. Which additional lazy files would activate for this file (subdirectory CLAUDE.md and path-scoped rules whose globs match the target)
3. Combined total token estimate

Documented limitation: this simulates directory membership and glob matching only. It cannot predict which collateral files Claude will read during agentic traversal.

### 5.7 Output Formats

**Terminal (default):** tree view, color-coded severity, token bars, load order annotations.

**`--json`:** structured JSON with all findings, file metadata, and token estimates. Schema defined in spec.

**`--quiet`:** no stdout output; exit codes only.
- `0` — no findings
- `1` — INFO only
- `2` — WARNING or ERROR present

**`--compose`:** appended to map output; prints the full concatenated eager context with source-file annotations as comments. Useful for diffing across branches or piping into other tools.

---

## 6. Out of Scope for v1

- Any write operations or fix suggestions
- Semantic conflict detection
- Scope-mismatch heuristics  
- Missing-coverage detection
- Watch mode / file system events
- Multi-directory / cross-repo reports
- Auto memory (`MEMORY.md`) analysis

These are tracked in `FUTURE.md`.

---

## 7. Constraints

- Zero runtime dependencies
- Works without `ANTHROPIC_API_KEY`
- Runs on macOS (arm64, amd64), Linux (amd64, arm64), Windows (amd64)
- Single binary distribution
- `claudemap map` completes in < 3 seconds on a project with 100 CLAUDE.md files

---

## 8. Success Criteria

| Metric | Target |
|--------|--------|
| Runtime on 100-file project | < 3 seconds |
| Token estimate accuracy vs. API count | within ±10% for typical CLAUDE.md content |
| False positive rate on dead-rule detection | < 5% |
| Issues correctly identified in test fixture suite | 100% |
