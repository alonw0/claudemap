# claudemap Hooks Setup

claudemap can integrate with Claude Code's hook system to automatically surface new CLAUDE.md issues at the start of your next session.

## How it works

1. **Stop hook** — runs `scripts/claudemap-suggest-updates` after every session. If new ERR or WARN findings appeared since the last run, it writes a short message to `.claude/claudemap-pending.md`.
2. **Start hook** — reads `.claude/claudemap-pending.md` at the beginning of the next session and injects it as context, then deletes the file.

Claude sees the pending message and proactively offers to help fix the issues before you even ask.

## Setup

### 1. Make the script available

Either install globally:
```bash
# build claudemap first
go build -o claudemap .
sudo cp claudemap /usr/local/bin/
sudo cp scripts/claudemap-suggest-updates /usr/local/bin/
```

Or use the project-relative path in the hook command (see below).

### 2. Add hooks to `.claude/settings.json`

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
    ],
    "PostToolUse": [],
    "PreToolUse": [],
    "Notification": [],
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

If `claudemap-suggest-updates` is not in your PATH, use the full path:
```json
"command": "/path/to/claudemap/scripts/claudemap-suggest-updates"
```

### 3. Add generated files to `.gitignore`

```
.claude/claudemap-baseline.json
.claude/claudemap-pending.md
```

## The analysis skill

To run a full semantic analysis of your CLAUDE.md setup (conflict detection, scope leakage, ordering surprises), install the skill:

```bash
cp .claude/skills/claudemap-analyze.md ~/.claude/skills/
```

Then in any Claude Code session:
```
/skill claudemap-analyze
```

Claude will run `claudemap check --json`, analyze the composed context, and propose targeted fixes.
