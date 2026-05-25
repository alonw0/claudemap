package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alonw0/claudemap/internal/assets"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the claudemap skill and/or session hooks into Claude Code",
	Long: `Install the claudemap analysis skill and session hooks into Claude Code.

By default, installs both the skill (globally) and hooks (into the current
project). Use --skill or --hooks to install only one, and --global/--local
to control where each is installed.`,
	Example: `  claudemap install                   # skill globally + hooks locally
  claudemap install --skill           # skill only, global (~/.claude/skills/)
  claudemap install --skill --local   # skill only, local (.claude/skills/)
  claudemap install --hooks           # hooks only, current project
  claudemap install --hooks --global  # hooks in ~/.claude/settings.json`,
	RunE: runInstall,
}

var (
	installSkill  bool
	installHooks  bool
	installGlobal bool
	installLocal  bool
)

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().BoolVar(&installSkill, "skill", false, "Install the claudemap-analyze skill")
	installCmd.Flags().BoolVar(&installHooks, "hooks", false, "Install Stop/Start session hooks")
	installCmd.Flags().BoolVar(&installGlobal, "global", false, "Install to ~/.claude/ (user-level)")
	installCmd.Flags().BoolVar(&installLocal, "local", false, "Install to .claude/ in the current project")
	installCmd.MarkFlagsMutuallyExclusive("global", "local")
}

func runInstall(cmd *cobra.Command, args []string) error {
	// Default: install both if neither flag given
	if !installSkill && !installHooks {
		installSkill = true
		installHooks = true
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	var errs []error

	if installSkill {
		// Default scope for skill: global
		skillDir := filepath.Join(home, ".claude", "skills")
		if installLocal {
			skillDir = filepath.Join(cwd, ".claude", "skills")
		}
		if err := installSkillFile(skillDir); err != nil {
			errs = append(errs, fmt.Errorf("skill: %w", err))
		}
	}

	if installHooks {
		// Default scope for hooks: local (project-level)
		settingsDir := filepath.Join(cwd, ".claude")
		if installGlobal {
			settingsDir = filepath.Join(home, ".claude")
		}
		if err := installHookSettings(settingsDir); err != nil {
			errs = append(errs, fmt.Errorf("hooks: %w", err))
		}
		if err := addGitignoreEntries(cwd); err != nil {
			errs = append(errs, fmt.Errorf("gitignore: %w", err))
		}
	}

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "error: %v\n", e)
		}
		return fmt.Errorf("installation incomplete")
	}

	fmt.Println("\nRestart Claude Code for hooks and skills to take effect.")
	return nil
}

func installSkillFile(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dst := filepath.Join(dir, "claudemap-analyze.md")
	if err := os.WriteFile(dst, assets.SkillContent, 0644); err != nil {
		return err
	}
	fmt.Printf("✓ skill  → %s\n", dst)
	return nil
}

func installHookSettings(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	settingsPath := filepath.Join(dir, "settings.json")

	// Load existing settings or start fresh
	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse %s: %w", settingsPath, err)
		}
	} else {
		settings = map[string]any{}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}

	stopCmd := "claudemap suggest-updates"
	startCmd := "cat .claude/claudemap-pending.md 2>/dev/null && rm -f .claude/claudemap-pending.md || true"

	mergeHook(hooks, "Stop", stopCmd)
	mergeHook(hooks, "Start", startCmd)

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, append(data, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("✓ hooks  → %s\n", settingsPath)
	return nil
}

// mergeHook adds cmd to the named hook list if not already present.
func mergeHook(hooks map[string]any, event, cmd string) {
	groups, _ := hooks[event].([]any)

	// Check if command already wired
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		items, _ := group["hooks"].([]any)
		for _, item := range items {
			h, ok := item.(map[string]any)
			if ok && h["command"] == cmd {
				return // already present
			}
		}
	}

	newGroup := map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{"type": "command", "command": cmd},
		},
	}
	hooks[event] = append(groups, newGroup)
}

func addGitignoreEntries(dir string) error {
	entries := []string{
		".claude/claudemap-baseline.json",
		".claude/claudemap-pending.md",
	}
	path := filepath.Join(dir, ".gitignore")

	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}

	added := false
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, entry := range entries {
		if containsLine(existing, entry) {
			continue
		}
		fmt.Fprintln(f, entry)
		added = true
	}
	if added {
		fmt.Printf("✓ .gitignore updated\n")
	}
	return nil
}

func containsLine(content, line string) bool {
	for _, l := range splitLines(content) {
		if l == line {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
