package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alonw0/claudemap/model"
)

func RunChecks(assembly *model.ContextAssembly, workdir string) []model.Finding {
	var findings []model.Finding
	counter := map[string]int{}

	newFinding := func(code model.FindingCode, sev model.Severity, file model.ClaudeFile, line int, msg, detail string) model.Finding {
		counter[string(code)]++
		prefix := map[model.Severity]string{
			model.SeverityError:   "E",
			model.SeverityWarning: "W",
			model.SeverityInfo:    "I",
		}[sev]
		id := fmt.Sprintf("%s-%s-%03d", prefix, string(code), counter[string(code)])
		return model.Finding{
			ID:       id,
			Code:     code,
			Severity: sev,
			File:     file,
			Line:     line,
			Message:  msg,
			Detail:   detail,
		}
	}

	allFiles := append(assembly.EagerFiles, assembly.LazyFiles...)

	// Collect all files under workdir for glob matching
	var workdirFiles []string
	filepath.WalkDir(workdir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workdir, path)
		if err == nil {
			workdirFiles = append(workdirFiles, filepath.ToSlash(rel))
		}
		return nil
	})

	// E01, E02, W03 — import issues
	var checkImports func(file model.ClaudeFile, refs []model.ImportRef)
	checkImports = func(file model.ClaudeFile, refs []model.ImportRef) {
		for _, ref := range refs {
			if ref.IsCircular {
				findings = append(findings, newFinding(
					model.CodeCircularImport, model.SeverityWarning, file, ref.Line,
					fmt.Sprintf("@%s — circular import detected", ref.Raw), ""))
			} else if ref.ExceedsDepth {
				findings = append(findings, newFinding(
					model.CodeImportDepth, model.SeverityError, file, ref.Line,
					fmt.Sprintf("@%s — import chain depth > 5; Claude silently drops this", ref.Raw),
					"Claude Code enforces a maximum import depth of 5 hops."))
			} else if !ref.Exists {
				findings = append(findings, newFinding(
					model.CodeBrokenImport, model.SeverityError, file, ref.Line,
					fmt.Sprintf("@%s → file not found", ref.Raw), ""))
			} else {
				checkImports(file, ref.Children)
			}
		}
	}

	for _, f := range allFiles {
		checkImports(f, f.Imports)
	}

	// W01, I01 — size issues
	for _, f := range allFiles {
		if f.LineCount > 200 {
			findings = append(findings, newFinding(
				model.CodeSizeViolation, model.SeverityWarning, f, 0,
				fmt.Sprintf("%d lines (recommended max: 200)", f.LineCount),
				"Consider extracting file-type-specific rules to .claude/rules/ with paths: frontmatter"))
		} else if f.LineCount >= 150 {
			findings = append(findings, newFinding(
				model.CodeSizeApproaching, model.SeverityInfo, f, 0,
				fmt.Sprintf("%d lines — approaching 200 line recommendation", f.LineCount), ""))
		}
	}

	// W02 — dead-rule (path-scoped rule matching 0 files)
	for _, f := range assembly.LazyFiles {
		if f.LoadTiming != model.LoadLazyRule || len(f.PathGlobs) == 0 {
			continue
		}
		matched := false
		for _, wf := range workdirFiles {
			if GlobMatchAny(f.PathGlobs, wf) {
				matched = true
				break
			}
		}
		if !matched {
			findings = append(findings, newFinding(
				model.CodeDeadRule, model.SeverityWarning, f, 0,
				fmt.Sprintf("paths: %v matches 0 files in this tree", f.PathGlobs),
				"This rule never loads. Remove it or update the glob."))
		}
	}

	// W04 — circular symlink (flagged during discovery)
	for _, f := range allFiles {
		if f.HasCircularSymlink {
			findings = append(findings, newFinding(
				model.CodeCircularSymlink, model.SeverityWarning, f, 0,
				"circular symlink detected in .claude/rules/", ""))
		}
	}

	// I02 — post-compaction-risk
	eagerText := assembledEagerText(assembly)
	for _, f := range assembly.LazyFiles {
		if f.LoadTiming != model.LoadLazyDir {
			continue
		}
		normalized := normalizeWhitespace(f.CleanContent)
		if !strings.Contains(normalizeWhitespace(eagerText), normalized) {
			findings = append(findings, newFinding(
				model.CodePostCompaction, model.SeverityInfo, f, 0,
				"rules in this file vanish after /compact until Claude opens a file in this directory",
				"Consider duplicating critical rules into the nearest eager CLAUDE.md"))
		}
	}

	// I03 — dead-exclude (claudeMdExcludes in settings.json)
	// Patterns match absolute paths per Claude Code docs.
	allDiscoveredPaths := make([]string, 0, len(allFiles))
	for _, f := range allFiles {
		allDiscoveredPaths = append(allDiscoveredPaths, filepath.ToSlash(f.Path))
	}
	var settingsFiles []string
	if home, _ := os.UserHomeDir(); home != "" {
		settingsFiles = append(settingsFiles,
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".claude", "settings.local.json"),
		)
	}
	settingsFiles = append(settingsFiles,
		filepath.Join(workdir, ".claude", "settings.json"),
		filepath.Join(workdir, ".claude", "settings.local.json"),
	)
	for _, settingsFile := range settingsFiles {
		checkDeadExcludes(settingsFile, workdir, allDiscoveredPaths, allFiles, &findings, newFinding)
	}

	return findings
}

func assembledEagerText(assembly *model.ContextAssembly) string {
	var sb strings.Builder
	for _, f := range assembly.EagerFiles {
		sb.WriteString(f.CleanContent)
		sb.WriteString("\n")
	}
	return sb.String()
}

func normalizeWhitespace(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

type settings struct {
	ClaudeMdExcludes []string `json:"claudeMdExcludes"`
}

func checkDeadExcludes(
	settingsFile, workdir string,
	allDiscoveredPaths []string,
	_ []model.ClaudeFile,
	findings *[]model.Finding,
	newFinding func(model.FindingCode, model.Severity, model.ClaudeFile, int, string, string) model.Finding,
) {
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		return
	}
	var s settings
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}

	// Create a fake ClaudeFile to attach findings to
	settingsRelPath, _ := filepath.Rel(workdir, settingsFile)
	settingsCF := model.ClaudeFile{Path: settingsFile, RelPath: settingsRelPath}

	for _, pattern := range s.ClaudeMdExcludes {
		matched := false
		for _, p := range allDiscoveredPaths {
			if GlobMatch(pattern, p) {
				matched = true
				break
			}
		}
		if !matched {
			*findings = append(*findings, newFinding(
				model.CodeDeadExclude, model.SeverityInfo, settingsCF, 0,
				fmt.Sprintf("claudeMdExcludes pattern %q matches no discovered CLAUDE.md files", pattern), ""))
		}
	}
}
