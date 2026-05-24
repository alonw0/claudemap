package render

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alonw0/claudemap/model"
)

var (
	colorReset  = "\033[0m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func init() {
	// Disable colors if NO_COLOR set or not a TTY
	if os.Getenv("NO_COLOR") != "" || !isTerminal(os.Stdout) {
		colorReset = ""
		colorDim = ""
		colorRed = ""
		colorYellow = ""
		colorBlue = ""
		colorCyan = ""
		colorBold = ""
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// Map renders the tree view to w.
func Map(w io.Writer, assembly *model.ContextAssembly, findings []model.Finding, simulateFile string, simulateExtra []model.ClaudeFile) {
	workdir := assembly.Workdir
	home, _ := os.UserHomeDir()

	friendlyPath := func(p string) string {
		// Prefer relative-to-workdir if it doesn't go up and isn't "."
		rel, err := filepath.Rel(workdir, p)
		if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return rel
		}
		// Fall back to tilde path
		if home != "" && strings.HasPrefix(p, home) {
			return "~" + p[len(home):]
		}
		return p
	}

	// Index findings by file path
	findingsByFile := map[string][]model.Finding{}
	for _, f := range findings {
		findingsByFile[f.File.Path] = append(findingsByFile[f.File.Path], f)
	}

	div := strings.Repeat("━", 68)

	fmt.Fprintf(w, "\n%sclaudemap map%s — %s\n\n", colorBold, colorReset, friendlyPath(workdir))

	// Header
	fmt.Fprintf(w, "%s%-56s %6s %8s%s\n", colorDim, "EAGER CONTEXT", "LINES", "TOKENS", colorReset)
	fmt.Fprintf(w, "%s%s%s\n", colorDim, div, colorReset)

	totalLines := 0
	totalTokens := 0

	for _, f := range assembly.EagerFiles {
		totalLines += f.LineCount
		totalTokens += f.Tokens

		label := scopeLabel(f.Scope)
		order := ""
		if f.LoadOrder > 0 {
			order = fmt.Sprintf("#%-2d", f.LoadOrder)
		} else {
			order = "#0 "
		}

		fp := friendlyPath(f.Path)
		fmt.Fprintf(w, "%s%s%s  %s[%s]%s  %-38s %6d %8s\n",
			colorDim, order, colorReset,
			colorCyan, label, colorReset,
			truncate(fp, 38),
			f.LineCount,
			fmt.Sprintf("~%d", f.Tokens))

		// Imports
		renderImports(w, f.Imports, "    ", friendlyPath)

		// Findings
		renderFileFindings(w, findingsByFile[f.Path], "    ")
	}

	fmt.Fprintf(w, "%s%s%s\n", colorDim, div, colorReset)
	fmt.Fprintf(w, "%s%-56s %6d %8s%s\n\n", colorDim,
		"TOTAL EAGER", totalLines, fmt.Sprintf("~%d", totalTokens), colorReset)

	// Lazy section
	if len(assembly.LazyFiles) > 0 {
		fmt.Fprintf(w, "%sLAZY (loads when matching files opened)%s\n", colorDim, colorReset)
		fmt.Fprintf(w, "%s%s%s\n", colorDim, div, colorReset)

		for _, f := range assembly.LazyFiles {
			fp := friendlyPath(f.Path)
			switch f.LoadTiming {
			case model.LoadLazyDir:
				fmt.Fprintf(w, "%s[dir]%s   %s\n", colorDim, colorReset, fp)
			case model.LoadLazyRule:
				globs := strings.Join(f.PathGlobs, ", ")
				fmt.Fprintf(w, "%s[rule]%s  %s  → paths: %s\n", colorDim, colorReset, fp, globs)
			}
			renderFileFindings(w, findingsByFile[f.Path], "        ")
		}
		fmt.Fprintln(w)
	}

	// Simulate section
	if simulateFile != "" && len(simulateExtra) > 0 {
		fmt.Fprintf(w, "%sADDITIONAL CONTEXT FOR %s%s\n", colorBold, simulateFile, colorReset)
		fmt.Fprintf(w, "%s%s%s\n", colorDim, div, colorReset)
		simTokens := 0
		simLines := 0
		for _, f := range simulateExtra {
			simTokens += f.Tokens
			simLines += f.LineCount
			switch f.LoadTiming {
			case model.LoadLazyDir:
				fmt.Fprintf(w, "%s[+dir]%s   %s   %6d %8s\n", colorDim, colorReset, friendlyPath(f.Path), f.LineCount, fmt.Sprintf("~%d", f.Tokens))
			case model.LoadLazyRule:
				globs := strings.Join(f.PathGlobs, ", ")
				fmt.Fprintf(w, "%s[+rule]%s  %s  (paths: %s)  %6d %8s\n", colorDim, colorReset, friendlyPath(f.Path), globs, f.LineCount, fmt.Sprintf("~%d", f.Tokens))
			}
		}
		fmt.Fprintf(w, "\n%s%-56s %6d %8s%s\n\n", colorDim,
			"TOTAL WITH SIMULATE", totalLines+simLines, fmt.Sprintf("~%d", totalTokens+simTokens), colorReset)
	}

	// Summary
	errs, warns, infos := countSeverities(findings)
	if errs+warns+infos == 0 {
		fmt.Fprintf(w, "%s✓ no issues found%s\n\n", colorCyan, colorReset)
	} else {
		parts := []string{}
		if errs > 0 {
			parts = append(parts, fmt.Sprintf("%s%d error(s)%s", colorRed, errs, colorReset))
		}
		if warns > 0 {
			parts = append(parts, fmt.Sprintf("%s%d warning(s)%s", colorYellow, warns, colorReset))
		}
		if infos > 0 {
			parts = append(parts, fmt.Sprintf("%d info", infos))
		}
		fmt.Fprintf(w, "%s\n\n", strings.Join(parts, " · "))
	}
}

func renderImports(w io.Writer, imports []model.ImportRef, indent string, friendlyPath func(string) string) {
	for _, imp := range imports {
		if imp.IsCircular {
			fmt.Fprintf(w, "%s%s└─ %s@%s%s %s(circular)%s\n",
				indent, colorYellow, colorReset, imp.Raw, colorReset, colorYellow, colorReset)
		} else if imp.ExceedsDepth {
			fmt.Fprintf(w, "%s%s└─ %s@%s%s %s(depth exceeded)%s\n",
				indent, colorRed, colorReset, imp.Raw, colorReset, colorRed, colorReset)
		} else if !imp.Exists {
			fmt.Fprintf(w, "%s%s└─ @%s → not found%s\n",
				indent, colorRed, imp.Raw, colorReset)
		} else {
			fp := friendlyPath(imp.Resolved)
			fmt.Fprintf(w, "%s%s└─ @%s%s %s(depth:%d)%s\n",
				indent, colorDim, fp, colorReset, colorDim, imp.Depth, colorReset)
			renderImports(w, imp.Children, indent+"   ", friendlyPath)
		}
	}
}

func renderFileFindings(w io.Writer, findings []model.Finding, indent string) {
	for _, f := range findings {
		icon, color := severityIcon(f.Severity)
		fmt.Fprintf(w, "%s%s%s %s %s%s\n",
			indent, color, icon, f.Code, f.Message, colorReset)
	}
}

// Check renders the check output.
func Check(w io.Writer, findings []model.Finding) {
	errs, warns, infos := countSeverities(findings)
	total := errs + warns + infos

	fmt.Fprintf(w, "\n%sclaudemap check%s — %d finding(s)\n\n", colorBold, colorReset, total)

	if total == 0 {
		fmt.Fprintf(w, "%s✓ no issues found%s\n\n", colorCyan, colorReset)
		return
	}

	for _, f := range findings {
		icon, color := severityIcon(f.Severity)
		fmt.Fprintf(w, "%s%s %s  %s%s\n", color, icon, f.Code, f.Severity, colorReset)
		loc := f.File.RelPath
		if loc == "" {
			loc = f.File.Path
		}
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, f.Line)
		}
		fmt.Fprintf(w, "   %s\n", loc)
		fmt.Fprintf(w, "   %s\n", f.Message)
		if f.Detail != "" {
			fmt.Fprintf(w, "   %s%s%s\n", colorDim, f.Detail, colorReset)
		}
		fmt.Fprintln(w)
	}
}

// Compose renders the full eager context.
func Compose(w io.Writer, assembly *model.ContextAssembly) {
	div := strings.Repeat("━", 68)
	fmt.Fprintf(w, "\n%s\nCOMPOSED EAGER CONTEXT\n%s\n\n", div, div)

	for _, block := range assembly.ComposedBlocks {
		fmt.Fprintf(w, "<!-- SOURCE: %s [load order: %d] -->\n",
			block.Source.RelPath, block.Source.LoadOrder)
		fmt.Fprintln(w, block.Content)
	}
}

func scopeLabel(s model.Scope) string {
	switch s {
	case model.ScopeManaged:
		return "managed"
	case model.ScopeUser:
		return "user   "
	case model.ScopeUserRule:
		return "user   "
	case model.ScopeProjectRoot:
		return "project"
	case model.ScopeProjectLocal:
		return "local  "
	case model.ScopeProjectRule:
		return "rule   "
	case model.ScopeSubdirectory:
		return "subdir "
	case model.ScopeSubdirLocal:
		return "subdir "
	default:
		return "unknown"
	}
}

func severityIcon(s model.Severity) (string, string) {
	switch s {
	case model.SeverityError:
		return "⛔", colorRed
	case model.SeverityWarning:
		return "⚠", colorYellow
	default:
		return "ⓘ", colorBlue
	}
}

func countSeverities(findings []model.Finding) (errs, warns, infos int) {
	for _, f := range findings {
		switch f.Severity {
		case model.SeverityError:
			errs++
		case model.SeverityWarning:
			warns++
		case model.SeverityInfo:
			infos++
		}
	}
	return
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-(n-1):]
}
