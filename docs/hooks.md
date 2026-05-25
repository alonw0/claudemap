# claudemap Hooks Setup

claudemap can integrate with Claude Code's hook system to automatically surface new CLAUDE.md issues at the start of your next session.

## How it works

1. **Stop hook** — runs `claudemap suggest-updates` after every session. If new ERR or WARN findings appeared since the last run, it writes a short message to `.claude/claudemap-pending.md`.
2. **Start hook** — reads `.claude/claudemap-pending.md` at the beginning of the next session and injects it as context, then deletes the file.

Claude sees the pending message and proactively offers to help fix the issues before you even ask.

## Quick install

```bash
claudemap install --hooks           # hooks into current project's .claude/settings.json
claudemap install --hooks --global  # hooks into ~/.claude/settings.json
claudemap install                   # hooks + skill in one step
```

`claudemap install` merges the Stop and Start hooks into `settings.json` (creates it if absent, preserves existing hooks), adds generated files to `.gitignore`, and optionally installs the analysis skill. Restart Claude Code afterward.

## Manual setup

### 1. Add hooks to `.claude/settings.json`

```json
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "claudemap suggest-updates"
          }
        ]
      }
    ],
    "Start": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "cat .claude/claudemap-pending.md 2>/dev/null && rm -f .claude/claudemap-pending.md || true"
          }
        ]
      }
    ]
  }
}
```

### 2. Add generated files to `.gitignore`

```
.claude/claudemap-baseline.json
.claude/claudemap-pending.md
```

## The analysis skill

To run a full semantic analysis of your CLAUDE.md setup (conflict detection, scope leakage, ordering surprises), install the skill:

```bash
claudemap install --skill
```

Then in any Claude Code session:
```
/skill claudemap-analyze
```

Claude will run `claudemap check --json`, analyze the composed context, and propose targeted fixes.
