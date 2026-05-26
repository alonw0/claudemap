package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alonw0/claudemap/analyze"
	"github.com/alonw0/claudemap/discover"
	"github.com/alonw0/claudemap/model"
	"github.com/alonw0/claudemap/render"
	"github.com/spf13/cobra"
)

var suggestUpdatesCmd = &cobra.Command{
	Use:    "suggest-updates",
	Short:  "Stop hook: detect new findings since last session and queue them for next start",
	Hidden: true, // Internal use — invoked by Claude Code Stop hook
	RunE:   runSuggestUpdates,
}

func init() {
	rootCmd.AddCommand(suggestUpdatesCmd)
}

func runSuggestUpdates(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil // silent — this runs as a hook
	}

	baselinePath := filepath.Join(cwd, ".claude", "claudemap-baseline.json")
	pendingPath := filepath.Join(cwd, ".claude", "claudemap-pending.md")

	// Run discovery + checks
	assembly, err := discover.Discover(cwd, discover.Opts{})
	if err != nil {
		return nil
	}
	findings := analyze.RunChecks(assembly, cwd)

	// Serialize current state for baseline storage
	currentJSON, err := render.JSON(assembly, findings)
	if err != nil {
		return nil
	}

	// Extract current ERR/WARN as sorted "sev:code:path" keys
	current := issueKeys(findings)

	// Load baseline
	baselineSet := map[string]bool{}
	if data, err := os.ReadFile(baselinePath); err == nil {
		var stored struct {
			Findings []struct {
				Severity string `json:"severity"`
				Code     string `json:"code"`
				FilePath string `json:"file_path"`
			} `json:"findings"`
		}
		if json.Unmarshal(data, &stored) == nil {
			for _, f := range stored.Findings {
				if f.Severity == "error" || f.Severity == "warning" {
					baselineSet[f.Severity+":"+f.Code+":"+f.FilePath] = true
				}
			}
		}
	}

	// Find new issues not in baseline
	var newIssues []string
	for _, k := range current {
		if !baselineSet[k] {
			newIssues = append(newIssues, k)
		}
	}

	// Write pending message if there are new issues.
	// Only advance the baseline if the notification was successfully delivered —
	// if the write fails, keep the baseline stale so the next session retries.
	notified := true
	if len(newIssues) > 0 {
		notified = false
		if err := os.MkdirAll(filepath.Dir(pendingPath), 0755); err == nil {
			var sb strings.Builder
			fmt.Fprintf(&sb, "## claudemap found %d new issue(s) since your last session\n\n", len(newIssues))
			for _, k := range newIssues {
				parts := strings.SplitN(k, ":", 3)
				if len(parts) == 3 {
					fmt.Fprintf(&sb, "- **%s** (`%s`) — `%s`\n", parts[1], parts[0], parts[2])
				}
			}
			sb.WriteString("\nSay **\"fix CLAUDE.md issues\"** to review and address these, or ignore this message.\n")
			if err := os.WriteFile(pendingPath, []byte(sb.String()), 0644); err == nil {
				notified = true
			}
		}
	}

	if notified {
		_ = os.MkdirAll(filepath.Dir(baselinePath), 0755)
		_ = os.WriteFile(baselinePath, currentJSON, 0644)
	}

	return nil
}

func issueKeys(findings []model.Finding) []string {
	var keys []string
	for _, f := range findings {
		if f.Severity == model.SeverityError || f.Severity == model.SeverityWarning {
			keys = append(keys, string(f.Severity)+":"+string(f.Code)+":"+f.File.Path)
		}
	}
	sort.Strings(keys)
	return keys
}
