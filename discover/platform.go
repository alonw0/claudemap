package discover

import "runtime"

func ManagedPolicyPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode/CLAUDE.md"
	case "windows":
		return `C:\Program Files\ClaudeCode\CLAUDE.md`
	default:
		return "/etc/claude-code/CLAUDE.md"
	}
}
