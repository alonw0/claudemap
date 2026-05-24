---
name: claudemap-analyze
description: Analyze CLAUDE.md configuration for rule conflicts, scope leakage, and ordering surprises using claudemap
---

## Steps

1. Run claudemap to get the current configuration:
   ```bash
   claudemap check --json
   ```
   If the command is not found, tell the user: "claudemap is not installed or not in PATH. Build it with `go build -o claudemap .` from the project root."

2. Parse the JSON output. If it contains ERROR findings (broken imports, circular symlinks), surface those first:
   > "claudemap found structural errors that should be fixed before semantic analysis: [list them]. Should I help fix these first, or proceed with the full analysis anyway?"

3. Extract `assembly.composed_blocks` (the full text each block contributes, in load order) and `findings` from the JSON. Proceed with the analysis below.

---

## Analysis Prompt

You are analyzing a Claude Code project's assembled CLAUDE.md configuration.

The context below shows every instruction Claude will receive, broken into blocks by source file and load order. **Load order matters: higher load-order numbers win when rules conflict** (files closer to the working directory load later and take precedence).

### What to look for

**CONFLICT** — Two rules that give opposite instructions for the same situation.
Examples of real conflicts:
- "always add semicolons" vs "never use semicolons"
- "prefer async/await" vs "use promise chains for readability"
- "run tests before committing" vs "skip tests on WIP commits"

Examples that are NOT conflicts (do not flag these):
- Same rule phrased differently in two files
- Rules that apply to different languages or file types
- Rules that are similar but not contradictory
- Redundant phrasing

**SCOPE** — A rule in a project file that expresses a personal preference (mentions "I", "my style", personal tooling) that belongs in `~/.claude/CLAUDE.md` instead. Flag sparingly — only clear cases.

**ORDERING** — A rule that is contradicted by a later-loading rule, where the user may not realize which one wins. State the effective winner clearly.

**OVERSPEC** — A rule in a broad eager file that is only relevant for one subdirectory or file type, wasting context for every session. Suggest moving to a path-scoped `.claude/rules/` file.

### Output format

For each genuine finding:

**[CONFLICT|SCOPE|ORDERING|OVERSPEC]** — one-line summary

Source A: `<file path>` (load order N)
> relevant excerpt (keep short)

Source B: `<file path>` (load order N)
> relevant excerpt (keep short)

**Effect**: What Claude will actually do (which rule wins, or whether behavior is unpredictable).

**Proposed fix**: Specific, minimal. Prefer removing or relocating over rewriting. If ambiguous, say so and ask which rule the user intends to keep.

---

If you find no genuine conflicts, say so clearly. Do not manufacture findings to seem thorough.

### The assembled context

<context>
[INSERT composed_blocks JSON here]
</context>

### Structural findings from claudemap

<findings>
[INSERT findings JSON here]
</findings>
