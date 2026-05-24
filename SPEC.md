# Technical Specification
# claudemap v2 — CLAUDE.md Context Analyzer

**Version:** 2.0  
**Status:** Draft  
**Last Updated:** 2026-05-24

---

## 1. Architecture

```
┌──────────────────────────────────────────────┐
│                  CLI Layer                   │
│   map | check [flags]                        │
└───────────────────┬──────────────────────────┘
                    │
┌───────────────────▼──────────────────────────┐
│               Orchestrator                   │
│  wires discovery → analysis → render         │
└──────┬──────────────┬───────────────┬────────┘
       │              │               │
┌──────▼──────┐ ┌─────▼──────┐ ┌─────▼──────┐
│  Discoverer │ │  Analyzer  │ │  Renderer  │
│             │ │            │ │            │
│ tree walks  │ │ issue rules│ │ terminal   │
│ import res. │ │ token est. │ │ JSON       │
│ glob match  │ │            │ │ compose    │
└─────────────┘ └────────────┘ └────────────┘
```

No global state. Each command instantiates a fresh pipeline. All structs are immutable after construction.

---

## 2. Data Model

```go
// ClaudeFile represents one discovered file in the ecosystem
type ClaudeFile struct {
    Path         string      // absolute
    RelPath      string      // relative to workdir
    Scope        Scope
    LoadTiming   LoadTiming
    LoadOrder    int         // position in eager sequence; 0 if lazy
    LineCount    int
    RawContent   string      // original file bytes as string
    CleanContent string      // HTML comments stripped
    Tokens       int         // estimated
    Imports      []ImportRef
    PathGlobs    []string    // from frontmatter, rules files only
    IsSymlink    bool
    SymlinkTarget string
}

type Scope int
const (
    ScopeManaged    Scope = iota // /etc/claude-code/CLAUDE.md etc.
    ScopeUser                    // ~/.claude/CLAUDE.md
    ScopeUserRule                // ~/.claude/rules/*.md
    ScopeProjectRoot             // ./CLAUDE.md or ./.claude/CLAUDE.md
    ScopeProjectLocal            // ./CLAUDE.local.md
    ScopeProjectRule             // ./.claude/rules/*.md
    ScopeSubdirectory            // ./foo/CLAUDE.md (lazy)
    ScopeSubdirLocal             // ./foo/CLAUDE.local.md (lazy)
)

type LoadTiming int
const (
    LoadEager    LoadTiming = iota // loaded at session start
    LoadLazyDir                    // loaded when Claude opens a file in that dir
    LoadLazyRule                   // loaded when Claude opens a file matching glob
)

type ImportRef struct {
    Raw          string      // the @path/to/file as written
    Resolved     string      // absolute path
    Depth        int         // 1 = direct import
    Exists       bool
    IsCircular   bool
    ExceedsDepth bool        // depth > 5
    Children     []ImportRef // recursive
}

// ContextAssembly is the ordered composed view of what Claude receives
type ContextAssembly struct {
    Workdir         string
    EagerFiles      []ClaudeFile    // in load order
    LazyFiles       []ClaudeFile
    EagerTokenTotal int
    EagerLineTotal  int
    ComposedBlocks  []ComposedBlock // for --compose output
}

type ComposedBlock struct {
    Source  ClaudeFile
    Content string      // the chunk contributed by this file (post comment strip)
    Tokens  int
}

// Finding is a single detected issue
type Finding struct {
    ID       string
    Code     FindingCode
    Severity Severity
    File     ClaudeFile
    Line     int    // 0 if not line-specific
    Message  string
    Detail   string // optional longer explanation
}

type FindingCode string
const (
    CodeBrokenImport      FindingCode = "broken-import"
    CodeImportDepth       FindingCode = "import-depth"
    CodeSizeViolation     FindingCode = "size-violation"
    CodeDeadRule          FindingCode = "dead-rule"
    CodeCircularImport    FindingCode = "circular-import"
    CodeCircularSymlink   FindingCode = "circular-symlink"
    CodeSizeApproaching   FindingCode = "size-approaching"
    CodePostCompaction    FindingCode = "post-compaction-risk"
    CodeDeadExclude       FindingCode = "dead-exclude"
)

type Severity string
const (
    SeverityError   Severity = "error"
    SeverityWarning Severity = "warning"
    SeverityInfo    Severity = "info"
)
```

---

## 3. Discovery

### 3.1 Upward Walk

Starting from `workdir`, walk to filesystem root. At each directory check for:
- `CLAUDE.md`
- `CLAUDE.local.md`
- `.claude/CLAUDE.md`
- `.claude/CLAUDE.local.md`

Collect all that exist. **Reverse the list** so filesystem-root files come first (they load first, have lower precedence). Within a single directory, `CLAUDE.md` before `CLAUDE.local.md`.

Assign `LoadOrder` starting at 1 after the managed and user files.

### 3.2 User-Level Files

Check in order:
1. `~/.claude/CLAUDE.md`
2. `~/.claude/rules/*.md` (recursive, unconditional rules first, then path-scoped)

These are prepended before the upward-walk results. They have lower `LoadOrder` than project files.

### 3.3 Managed Policy

Platform-specific lookup:
```
darwin:  /Library/Application Support/ClaudeCode/CLAUDE.md
linux:   /etc/claude-code/CLAUDE.md
windows: C:\Program Files\ClaudeCode\CLAUDE.md
```

If found, this gets `LoadOrder = 0` (loads before everything).

### 3.4 Downward Walk

Walk all subdirectories of workdir. For each directory that is not workdir itself, check for `CLAUDE.md` and `CLAUDE.local.md`. Mark these as `LoadLazyDir`. Do not assign a `LoadOrder` (lazy files have none).

Skip directories matching a default ignore list:
```
node_modules/  .git/  vendor/  dist/  build/  .next/  __pycache__/  target/
```

Respect `.gitignore` files during traversal if the `--respect-gitignore` flag is set (default: true).

### 3.5 Rules Discovery

For `.claude/rules/` in workdir (and `~/.claude/rules/`):

1. Walk recursively for all `.md` files
2. For each file, attempt symlink resolution with cycle detection (track inodes seen)
3. Parse YAML frontmatter:
   ```yaml
   ---
   paths:
     - "src/api/**/*.ts"
   ---
   ```
4. If `paths` present: `LoadLazyRule`, store globs
5. If no `paths`: `LoadEager`, treat like `.claude/CLAUDE.md`

### 3.6 Import Resolution

```
resolveImports(file, depth, seen):
  for each line in file.RawContent:
    if line matches /^@(.+)$/ or contains standalone @reference:
      rawPath = captured group
      absPath = resolve(rawPath, relative_to=dir(file.Path))
      
      if absPath in seen:
        append ImportRef{IsCircular: true}
        continue
      
      if depth >= 5:
        append ImportRef{ExceedsDepth: true}
        continue
        
      if not exists(absPath):
        append ImportRef{Exists: false}
        continue
      
      seen.add(absPath)
      child = parseFile(absPath)
      children = resolveImports(child, depth+1, seen)
      append ImportRef{Resolved: absPath, Depth: depth, Children: children}
```

`@` references can appear anywhere in a line — match the pattern `@[^\s]+` and resolve as paths relative to the containing file's directory.

---

## 4. Load Order Assembly

The complete load order, from first-in-context to last-in-context:

```
1.  Managed policy CLAUDE.md                    (LoadOrder 1)
2.  ~/.claude/CLAUDE.md                         (LoadOrder 2)
3.  ~/.claude/rules/*.md  (no paths frontmatter) (LoadOrder 3+)
4.  Ancestor CLAUDE.md files (root → parent)    (LoadOrder N...)
5.  ./CLAUDE.md or ./.claude/CLAUDE.md          
6.  ./CLAUDE.local.md                           (same level, after)
7.  ./.claude/rules/*.md (no paths frontmatter) (LoadOrder N+1...)
```

Within step 4, ordering is root-first so that closer-to-workdir files appear later (and thus have effective precedence for conflicting instructions).

Lazy files are never included in the composed context. They are listed separately in the tree output.

---

## 5. Token Estimation

```go
func estimateTokens(raw string) int {
    // Step 1: strip HTML comments outside code blocks
    clean := stripHTMLComments(raw)
    
    // Step 2: find fenced code blocks
    codeBlockRe := regexp.MustCompile("(?s)```[\\s\\S]*?```")
    codeBlocks := codeBlockRe.FindAllString(clean, -1)
    
    codeChars := 0
    for _, b := range codeBlocks {
        codeChars += len(b)
    }
    proseChars := len(clean) - codeChars
    
    // Step 3: estimate
    // Prose/markdown: ~4 chars/token
    // Code: ~3.5 chars/token (symbols, paths, short identifiers tokenize denser)
    tokens := float64(proseChars)/4.0 + float64(codeChars)/3.5
    
    return int(math.Ceil(tokens))
}

func stripHTMLComments(s string) string {
    // Must preserve comments inside code blocks
    // Strategy: replace code blocks with placeholder, strip comments, restore
    codeBlockRe := regexp.MustCompile("(?s)```[\\s\\S]*?```")
    
    placeholders := map[string]string{}
    i := 0
    result := codeBlockRe.ReplaceAllStringFunc(s, func(match string) string {
        key := fmt.Sprintf("__CODEBLOCK_%d__", i)
        placeholders[key] = match
        i++
        return key
    })
    
    commentRe := regexp.MustCompile("(?s)<!--[\\s\\S]*?-->")
    result = commentRe.ReplaceAllString(result, "")
    
    for key, val := range placeholders {
        result = strings.ReplaceAll(result, key, val)
    }
    return result
}
```

**Accuracy note:** For typical CLAUDE.md content (English instructions, light markdown, occasional shell commands), this estimates within ±8% of actual Claude API token counts. For files heavy in dense code or glob patterns, error may reach ±15%. All output labels estimates as `~N tokens`.

---

## 6. Issue Detection

### E01 — broken-import
```
for each ImportRef in any ClaudeFile where Exists == false:
  emit Finding{Code: CodeBrokenImport, Severity: SeverityError,
    Line: lineOfImportRef,
    Message: fmt("@%s → file not found", ref.Raw)}
```

### E02 — import-depth
```
for each ImportRef where ExceedsDepth == true:
  emit Finding{Code: CodeImportDepth, Severity: SeverityError,
    Message: fmt("@import chain depth > 5; Claude silently drops this")}
```

### W01 — size-violation
```
for each ClaudeFile where LineCount > 200:
  emit Finding{Code: CodeSizeViolation, Severity: SeverityWarning,
    Message: fmt("%d lines (recommended max: 200)", file.LineCount)}
```

### W02 — dead-rule
```
for each ClaudeFile where LoadTiming == LoadLazyRule:
  matches = globMatch(file.PathGlobs, allFilesUnderWorkdir)
  if len(matches) == 0:
    emit Finding{Code: CodeDeadRule, Severity: SeverityWarning,
      Message: fmt("paths: %v matches 0 files", file.PathGlobs)}
```

Glob matching uses the same syntax as documented (standard `**` globbing). Use `filepath.Match` extended with `**` support, or a minimal glob library embedded in-source (< 200 lines, zero external deps).

### W03 — circular-import
```
for each ImportRef where IsCircular == true:
  emit Finding{Code: CodeCircularImport, Severity: SeverityWarning}
```

### W04 — circular-symlink
```
// Detected during rules discovery via inode tracking
for each symlink where cycle detected:
  emit Finding{Code: CodeCircularSymlink, Severity: SeverityWarning}
```

### I01 — size-approaching
```
for each ClaudeFile where LineCount >= 150 and LineCount <= 200:
  emit Finding{Code: CodeSizeApproaching, Severity: SeverityInfo}
```

### I02 — post-compaction-risk
```
for each ClaudeFile where LoadTiming == LoadLazyDir:
  eagerContent = assembledEagerText()
  for each rule/section in file:
    if not substantiallySimilar(rule, eagerContent):
      emit Finding{Code: CodePostCompaction, Severity: SeverityInfo,
        Message: "rules here vanish after /compact until Claude reads a file in this directory",
        Detail: "consider duplicating critical rules into the nearest eager CLAUDE.md"}
      break  // one finding per file, not per rule
```

`substantiallySimilar` is a simple substring check (normalized whitespace). Not semantic. This will have false positives on rules that are genuinely unique to a subdirectory — that's acceptable for INFO severity.

### I03 — dead-exclude
```
for each settingsFile in [".claude/settings.json", ".claude/settings.local.json"]:
  if exists(settingsFile):
    settings = parseJSON(settingsFile)
    for each pattern in settings.claudeMdExcludes:
      absPattern = resolveAbsoluteGlob(pattern)
      if countMatches(absPattern, allDiscoveredClaudeFiles) == 0:
        emit Finding{Code: CodeDeadExclude, Severity: SeverityInfo}
```

---

## 7. Terminal Renderer

### 7.1 Map Output

```
claudemap map — /home/user/myproject/src/api

EAGER CONTEXT                                       LINES    TOKENS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#1  [managed]  /etc/claude-code/CLAUDE.md              12      ~48
#2  [user]     ~/.claude/CLAUDE.md                     34     ~136
#3  [user]     ~/.claude/rules/preferences.md           8      ~32
#4  [project]  ~/myproject/CLAUDE.md                  180     ~720
               └─ @docs/conventions.md (depth:1)       45     ~180
#5  [project]  ~/myproject/src/CLAUDE.md               41     ~164
#6  [local]    ~/myproject/src/CLAUDE.local.md          9      ~36
#7  [project]  ~/myproject/src/api/CLAUDE.md           38     ~152
               ⚠ W01 247 lines — exceeds 200 line recommendation
               
               TOTAL EAGER                            367   ~1,468

LAZY (loads when matching files opened)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[dir]   ~/myproject/src/api/handlers/CLAUDE.md
        ⓘ I02 post-compaction-risk
[rule]  .claude/rules/testing.md  → paths: **/*.test.ts  (12 matches)
[rule]  .claude/rules/php.md      → paths: **/*.php  ✗ no matches
        ⚠ W02 dead-rule

2 warnings · 1 info  — run `claudemap check` for details
```

Colors:
- `✗` and `⚠` in yellow for warnings
- `✗` and `⛔` in red for errors
- `ⓘ` in blue for info
- Load-order numbers in dim/gray
- File paths in default
- Token counts right-aligned

### 7.2 Check Output

```
claudemap check — 3 findings

⚠ W01  size-violation
   ~/myproject/src/api/CLAUDE.md — 247 lines (recommended max: 200)
   Consider extracting file-type-specific rules to .claude/rules/ with paths: frontmatter

⚠ W02  dead-rule
   .claude/rules/php.md — paths: **/*.php matches 0 files in this tree
   This rule never loads. Remove it or update the glob.

ⓘ I02  post-compaction-risk
   ~/myproject/src/api/handlers/CLAUDE.md
   Rules in this file vanish after /compact and only reload when Claude opens
   a file in handlers/. If any rules here are critical, duplicate them in
   ~/myproject/src/api/CLAUDE.md (eager).

exit 2
```

---

## 8. JSON Output Schema

`claudemap check --json` emits a single JSON object to stdout:

```json
{
  "claudemap_version": "2.0.0",
  "workdir": "/home/user/myproject/src/api",
  "timestamp": "2026-05-24T12:00:00Z",
  "assembly": {
    "eager_token_total": 1468,
    "eager_line_total": 367,
    "eager_files": [
      {
        "path": "/home/user/myproject/CLAUDE.md",
        "rel_path": "CLAUDE.md",
        "scope": "project_root",
        "load_timing": "eager",
        "load_order": 4,
        "line_count": 180,
        "tokens": 720,
        "imports": [
          {
            "raw": "@docs/conventions.md",
            "resolved": "/home/user/myproject/docs/conventions.md",
            "depth": 1,
            "exists": true,
            "is_circular": false,
            "exceeds_depth": false
          }
        ],
        "path_globs": null
      }
    ],
    "lazy_files": []
  },
  "findings": [
    {
      "id": "W01-001",
      "code": "size-violation",
      "severity": "warning",
      "file_path": "/home/user/myproject/src/api/CLAUDE.md",
      "line": 0,
      "message": "247 lines (recommended max: 200)",
      "detail": "Consider extracting file-type-specific rules to .claude/rules/ with paths: frontmatter"
    }
  ],
  "summary": {
    "total_findings": 3,
    "errors": 0,
    "warnings": 2,
    "info": 1
  }
}
```

---

## 9. HTML Report Renderer

`claudemap check --report html` writes a self-contained single-file HTML report to `claudemap-report.html` in the current directory (override with `--output <path>`).

### Design

Dark-theme IDE aesthetic. Three-column layout: topbar + sidebar nav + main content area. All CSS and JS is inlined — no external dependencies, no CDN calls, works fully offline.

Font stack: `IBM Plex Mono` for all code/labels/numbers (loaded from Google Fonts on first open if online; falls back to system monospace offline), `IBM Plex Sans` for body text.

### Layout Structure

```
┌─────────────────────────────────────────────────────────┐
│  TOPBAR: logo · workdir path · badge summary            │
├────────────────┬────────────────────────────────────────┤
│  SIDEBAR       │  MAIN CONTENT                          │
│                │                                        │
│  Views:        │  Active panel renders here             │
│  · Overview    │                                        │
│  · Context Tree│                                        │
│  · Composed    │                                        │
│  · Findings    │                                        │
│                │                                        │
│  File list     │                                        │
│  (eager/lazy)  │                                        │
└────────────────┴────────────────────────────────────────┘
```

### Four Panels

**Overview** — default view on open.
- Summary stat cells: total eager tokens, total eager lines, warning count, error count
- Token composition bar: proportional horizontal bar with one color segment per file, labeled legend below
- Findings summary list (all findings, one per row, severity icon + message + file)

**Context Tree** — full file tree with columns.

Column layout per row:
```
[load#]  [icon]  [file path]  [scope badge]  [timing]  [lines]  [~tokens]  [finding dots]
```
- Eager files numbered `#1 … #N`, lazy files show type (`dir` / `rule`)
- `@import` sub-rows indented under their parent with depth badge and ✓/✗ indicator
- Scope badges color-coded: managed=purple, user=gray, project=blue, local=gray-outlined, lazy=dim, rule=green
- Dead rules highlighted in red throughout the row
- Section dividers separating eager from lazy blocks
- Totals row at bottom of eager section

**Composed Context** — accordion view.
- One collapsible block per eager file (plus imported files inline)
- Header shows: load order number, file path, scope label, token count, expand toggle
- Expanded body shows the clean content (HTML comments stripped) in a monospace pre block
- Collapsed by default; user opens what they want to read

**Findings** — full findings list with tab filter.
- Tabs: All / Errors / Warnings / Info (counts in tab label)
- Each finding: severity badge, finding code, human message, file path + line, detail paragraph
- Filtering hides/shows rows client-side, no page reload

### Self-Containment Requirements

The HTML file must work when:
- Opened directly from the filesystem (`file://` protocol)
- Emailed or shared as an attachment
- Viewed without internet access (fonts degrade gracefully)

No `fetch()` calls, no external script tags, no absolute asset paths. All interactivity via inline `<script>`.

### Generation

The renderer receives the same `ContextAssembly` and `[]Finding` structs used by the terminal and JSON renderers. It templates them into the HTML using Go's `html/template`. All user-supplied content (file paths, rule text, finding messages) is HTML-escaped by the template engine.

The composed content blocks use the `CleanContent` field (HTML comments stripped, code blocks preserved) formatted inside `<pre>` tags with HTML escaping applied.

### Output

Written to disk, not stdout. After writing, print to terminal:
```
report written → claudemap-report.html
```

---

## 10. Simulate Mode

`claudemap map --simulate-open <target>`

```
1. Run normal discovery and assembly for workdir
2. Resolve target to absolute path
3. Collect additional files that would load:
   a. LazyDir files: any ClaudeFile where LoadTiming==LoadLazyDir
      and target is under that file's directory
   b. LazyRule files: any ClaudeFile where LoadTiming==LoadLazyRule
      and globMatch(file.PathGlobs, [target]) == true
4. Display normal map, then:

ADDITIONAL CONTEXT FOR src/api/handlers/payments.ts
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[+dir]  ~/myproject/src/api/handlers/CLAUDE.md        38    ~152
[+rule] .claude/rules/api-design.md  (paths: src/api/**/*.ts)  22  ~88

TOTAL WITH SIMULATE                                  427   ~1,708
```

---

## 11. Compose Mode

`claudemap map --compose` appends the full assembled eager context after the tree:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
COMPOSED EAGER CONTEXT
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

<!-- SOURCE: ~/.claude/CLAUDE.md [load order: 2] -->
[file content here]

<!-- SOURCE: ~/myproject/CLAUDE.md [load order: 4] -->
[file content here]

<!-- IMPORT: docs/conventions.md [depth: 1, from: CLAUDE.md] -->
[imported file content here]

...
```

HTML comments are used as source annotations so the output is itself valid markdown and doesn't confuse downstream tools. The content uses `CleanContent` (HTML comments stripped from original, code blocks preserved).

---

## 12. Implementation Language: Go

**Reasons:**
- Single binary, no runtime install, trivial cross-compilation
- `os`, `path/filepath`, `regexp`, `encoding/json` cover 95% of needs from stdlib
- Fast startup (important for CLI feel)
- Simple concurrency if directory walking needs parallelism later

**Key stdlib packages used:**
- `os.ReadFile`, `os.Stat`, `os.Lstat` — file reading and symlink detection
- `filepath.WalkDir` — directory traversal
- `filepath.Rel`, `filepath.Abs` — path resolution
- `regexp` — glob simulation, import detection, comment stripping
- `encoding/json` — JSON output
- `gopkg.in/yaml.v3` — ONLY external dependency, for frontmatter parsing (or implement a minimal frontmatter parser in ~50 lines to stay truly zero-dep)

**Recommended project layout:**
```
claudemap/
├── main.go
├── cmd/
│   ├── map.go
│   └── check.go
├── discover/
│   ├── discover.go      // all discovery logic
│   ├── imports.go       // import resolution
│   └── platform.go      // managed policy paths
├── analyze/
│   ├── tokens.go        // token estimation
│   └── check.go         // all issue detectors
├── render/
│   ├── terminal.go
│   └── json.go
└── testdata/
    ├── simple/
    ├── monorepo/
    ├── broken-imports/
    ├── deep-imports/
    ├── dead-rules/
    └── circular/
```

---

## 13. Testing

### Fixture Corpus

Each fixture is a directory tree with a `want.json` alongside it — the expected `claudemap check --json` output. Tests run `claudemap check --json` against the fixture and diff against `want.json`.

| Fixture | Tests |
|---------|-------|
| `simple/` | single CLAUDE.md, no issues |
| `eager-only/` | root + user + managed, correct load order |
| `deep-imports/` | 4-hop chain (valid), 6-hop chain (E02) |
| `broken-imports/` | missing file (E01) |
| `circular-imports/` | circular @reference (W03) |
| `dead-rules/` | path-scoped rule with no matching files (W02) |
| `size-violations/` | file over 200 lines (W01), file 155 lines (I01) |
| `post-compaction/` | subdirectory-only rule (I02) |
| `symlinks/` | valid symlink in rules/, circular symlink (W04) |
| `monorepo/` | multiple teams' files, claudeMdExcludes (I03) |
| `compose/` | --compose output matches expected assembled text |

### Token Estimator Tests

Test the estimator against a small set of known texts where the exact API count is known. Assert all estimates are within ±15% of actual. Include:
- Pure prose paragraph
- File with a large fenced code block
- File heavy in glob patterns and shell commands
- File with HTML comments (assert comments are excluded from count)

### Unit Tests

- `stripHTMLComments`: preserves comments inside code blocks, strips outside
- Import resolver: all edge cases (missing, circular, depth 6, relative paths)  
- Glob matching: `**/*.ts` matches `src/api/foo.ts` but not `src/api/foo.js`
- Load order: given 5 ancestor directories, result is root-first
- Platform paths: correct managed policy path per OS
