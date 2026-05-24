# FUTURE.md
# claudemap — Phase 2: Conflict Detection via Claude Code Skill + Hook

**Status:** Planning  
**Depends on:** claudemap v1 (map + check) being stable

---

## The Idea

claudemap v1 is a static analyzer — it finds structural problems that can be detected by reading files. It deliberately avoids semantic analysis because semantic analysis requires intelligence, not just parsing.

Phase 2 flips the model: instead of claudemap trying to understand what rules mean, we bring the analysis *inside* Claude Code, where Claude can read the files directly and reason about them. claudemap's job becomes generating a clean report; a Claude Code skill and hook turn that report into actionable fixes.

This means:
- No AI API calls from the claudemap binary itself — it stays zero-dependency
- Analysis runs in a Claude Code session, where Claude already has full context
- Fixes are proposed inside the same session, so Claude can apply them directly

---

## Component 1: Enhanced claudemap Output

Before phase 2 can work, claudemap needs one addition to its JSON output: the full composed text of each file, annotated with source metadata. Phase 1 already has `--compose` for human reading; phase 2 needs a machine-readable version of the same thing.

Add to `claudemap check --json`:

```json
{
  "assembly": {
    "eager_files": [...],
    "composed_blocks": [
      {
        "source_file": "~/.claude/CLAUDE.md",
        "scope": "user",
        "load_order": 2,
        "content": "... clean content ..."
      },
      {
        "source_file": "~/myproject/CLAUDE.md",
        "scope": "project_root",
        "load_order": 4,
        "content": "... clean content ..."
      }
    ]
  }
}
```

This is the raw material the skill will analyze.

---

## Component 2: The Analysis Skill

A Claude Code skill (`.claude/skills/claudemap-analyze.md` or installed via plugin) that:

1. Runs `claudemap check --json` as a shell command and captures output
2. Sends the findings + composed context to Claude with a structured prompt
3. Returns a human-readable analysis with specific conflict findings and fix proposals

### Invoking the skill

The user invokes it explicitly:
```
/skill claudemap-analyze
```
or shorthand:
```
analyze my CLAUDE.md setup
```

### The Prompt

The prompt is the most important part. It needs to be carefully engineered to minimize false positives and produce actionable output.

```markdown
# CLAUDE.md Configuration Analyzer

You are analyzing a Claude Code project's CLAUDE.md configuration.
You have been given the complete assembled context that Claude will load,
broken into blocks with their source files and load order.

## Your task

Identify ONLY genuine problems — instructions that will cause Claude to behave
inconsistently or unexpectedly. Do not flag style differences or redundant
phrasing unless they directly contradict each other.

## What to look for

**Contradictions**: Two rules that give opposite instructions for the same
situation. Example: "always add semicolons" (file A) vs "never use semicolons"
(file B). These are real problems.

**Scope leakage**: A rule in a project-level file that clearly expresses a
personal preference (mentions "I", "my style", specific personal tooling) that
should be in ~/.claude/CLAUDE.md instead. Flag sparingly — only clear cases.

**Ordering surprises**: A rule in a high-load-order file (closer to root, lower
precedence) that is contradicted by a rule in a lower-load-order file (closer
to workdir, higher precedence). The user may not realize which one wins. Note
the effective winner.

**Over-specification**: A rule in a broad eager file that is actually only
relevant for one subdirectory or file type, wasting context for all sessions.
Suggest moving to a path-scoped rule instead.

## What NOT to flag

- Rules that are similar but not contradictory
- Redundant phrasing (saying the same thing two ways)
- Aesthetic preferences that don't conflict
- Missing rules (you don't know what the user intended)
- Rules that are vague (vagueness is not a bug)

## Output format

For each genuine finding, output:

**[CONFLICT|SCOPE|ORDERING|OVERSPEC]** — one-line summary

Source A: `<file path>` (load order N)
> relevant excerpt

Source B: `<file path>` (load order N)  
> relevant excerpt

**Effect**: What Claude will actually do when these conflict (which rule wins, or
whether behavior is unpredictable).

**Proposed fix**: Specific, minimal change. Prefer removing or relocating over
rewriting. If the fix is ambiguous (you don't know which rule the user intends),
say so and ask.

---

If you find no genuine conflicts, say so clearly. Do not manufacture findings.

## The assembled context

<context>
{{INSERT_COMPOSED_BLOCKS_JSON}}
</context>

## Structural findings from claudemap

<findings>
{{INSERT_CLAUDEMAP_FINDINGS_JSON}}
</findings>
```

### Skill implementation

```markdown
---
name: claudemap-analyze
description: Analyze CLAUDE.md configuration for conflicts and issues
---

## Steps

1. Run claudemap to get the current configuration analysis:
   ```bash
   claudemap check --json
   ```
   
2. If claudemap is not installed, inform the user and stop:
   "claudemap is not installed. Install it from https://github.com/yourorg/claudemap"

3. Parse the JSON output. If it contains errors (broken imports, etc.), surface
   those first and ask if the user wants to fix them before semantic analysis.

4. Proceed with the full analysis prompt using the composed_blocks and findings
   from the JSON output.

5. Present findings to the user. For each fix proposed:
   - Show exactly what would change (diff format if possible)
   - Ask for confirmation before making any edit
   - Apply changes one at a time, not in bulk
```

---

## Component 3: The Post-Session Hook

The skill above is on-demand. The hook makes improvement automatic — it fires after every session and quietly proposes CLAUDE.md improvements while context is fresh.

This is a Claude Code **stop hook** — it runs after Claude finishes a session.

### Hook configuration (`.claude/settings.json`)

```json
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "claudemap-suggest-updates"
          }
        ]
      }
    ]
  }
}
```

### What the hook does

`claudemap-suggest-updates` is a small script (shell or Go) that:

1. Runs `claudemap check --json` and checks if there are new ERROR or WARNING findings since the last run (compares against a stored baseline in `.claude/claudemap-baseline.json`)
2. If new issues found: writes a summary to `.claude/claudemap-pending.md`
3. On the next session start (via a corresponding Start hook), Claude reads `claudemap-pending.md` and offers to address the issues

This avoids interrupting the current session — the suggestion appears at the beginning of the next one, when context is fresh.

### Start hook (reads pending suggestions)

```json
{
  "hooks": {
    "Start": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command", 
            "command": "cat .claude/claudemap-pending.md 2>/dev/null && rm -f .claude/claudemap-pending.md"
          }
        ]
      }
    ]
  }
}
```

When `.claude/claudemap-pending.md` exists, its content is injected into the session start. Claude sees it as context and can proactively offer to fix the issues before the user even asks.

### The pending message format

```markdown
## claudemap found issues since your last session

The following CLAUDE.md configuration issues were detected:

**W01 size-violation** — `src/api/CLAUDE.md` is now 247 lines (recommended max: 200)
**W02 dead-rule** — `.claude/rules/php.md` paths glob matches no files

Would you like me to help address these? I can:
1. Split `src/api/CLAUDE.md` by extracting API-specific rules to a path-scoped rules file
2. Remove or update the dead `php.md` rule

Say "fix CLAUDE.md issues" to proceed, or ignore this if you'll handle it later.
```

---

## Component 4: The InstructionsLoaded Hook (Debug Mode)

Claude Code fires an `InstructionsLoaded` hook when it loads memory files. This is the closest thing to ground truth for what actually loaded.

A debug hook can log the exact files Claude loaded to a file, which claudemap can then compare against its own prediction:

```json
{
  "hooks": {
    "InstructionsLoaded": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "echo $CLAUDE_INSTRUCTIONS_LOADED >> .claude/loaded.log"
          }
        ]
      }
    ]
  }
}
```

This is useful for:
- Verifying claudemap's discovery is correct
- Debugging cases where a rule seems to not be loading
- Building a test oracle to validate claudemap's output against reality

The environment variable format for `CLAUDE_INSTRUCTIONS_LOADED` is not yet documented — this would need verification once Claude Code exposes it.

---

## Prompt Engineering Notes

The system prompt above is a starting point. Known failure modes to test against:

**False positives to eliminate:**
- "use meaningful variable names" (projectA) + "don't over-engineer" (projectB) — not a conflict
- Same rule phrased differently in two files — not a conflict
- Rule that applies to a subset of files that the other rule doesn't address — not a conflict

**True positives to catch:**
- "always use tabs" vs "use 2-space indentation"
- "never add comments to obvious code" vs "add comments explaining every function"  
- "prefer async/await" vs "use promise chains for readability"
- "run tests before committing" (user file) vs "skip tests on WIP commits" (project file)

**The hardest cases:**
- Intentional specialization: a subdirectory rule that overrides the root rule on purpose. The tool should note this as a potential ordering surprise, not flag it as a bug.
- Rules that conflict only in specific contexts: "use single quotes" (for Python) vs "use double quotes" (for JSON). If scoping is clear from context, not a conflict.

The prompt should ask Claude to reason about these cases before flagging, not just pattern-match on surface similarity.

---

## Phase 2 Rollout Order

1. **Add `composed_blocks` to claudemap JSON output** — purely additive, no breaking changes
2. **Build and test the skill** — works standalone, no hook needed
3. **Add the stop + start hooks** — opt-in, users add to their settings manually
4. **Package as a Claude Code plugin** — skill + hooks distributed as one installable unit
5. **InstructionsLoaded validation** — once the hook env var is documented

---

## What Phase 2 Does Not Do

- It does not auto-apply fixes. Claude proposes; the user confirms.
- It does not run on every file save. The hook fires after a full session, not continuously.
- It does not replace reading your own CLAUDE.md files. The goal is catching things you miss, not thinking for you.
- It does not need a dedicated API key beyond what Claude Code already uses.
