package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alonw0/claudemap/analyze"
	"github.com/alonw0/claudemap/discover"
	"github.com/alonw0/claudemap/model"
	"github.com/alonw0/claudemap/render"
)

var (
	mapWorkdir      string
	mapCompose      bool
	mapSimulateOpen string
	mapNoGitignore  bool
	mapReport       string
	mapOutput       string
)

func init() {
	mapCmd.Flags().StringVar(&mapWorkdir, "workdir", "", "Run against a different directory (default: current directory)")
	mapCmd.Flags().BoolVar(&mapCompose, "compose", false, "Print the full composed eager context after the tree")
	mapCmd.Flags().StringVar(&mapSimulateOpen, "simulate-open", "", "Show what additional context loads when Claude opens this file")
	mapCmd.Flags().BoolVar(&mapNoGitignore, "no-gitignore", false, "Ignore .gitignore files during traversal")
	mapCmd.Flags().StringVar(&mapReport, "report", "", "Report format: html")
	mapCmd.Flags().StringVar(&mapOutput, "output", "claudemap-report.html", "Output path for --report html")
	rootCmd.AddCommand(mapCmd)
}

var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Show the context tree for the current directory",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		workdir := mapWorkdir
		if workdir == "" {
			var err error
			workdir, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		opts := discover.Opts{RespectGitignore: !mapNoGitignore}
		assembly, err := discover.Discover(workdir, opts)
		if err != nil {
			return fmt.Errorf("discovery failed: %w", err)
		}

		findings := analyze.RunChecks(assembly, workdir)

		var simulateExtra []model.ClaudeFile
		simulateTarget := ""
		if mapSimulateOpen != "" {
			simulateTarget = mapSimulateOpen
			abs, err := filepath.Abs(mapSimulateOpen)
			if err == nil {
				simulateTarget = abs
			}
			simulateExtra = collectSimulateFiles(assembly, simulateTarget)
		}

		if mapReport == "html" {
			out := mapOutput
			if !filepath.IsAbs(out) {
				wd, _ := os.Getwd()
				out = filepath.Join(wd, out)
			}
			return render.HTML(assembly, findings, out)
		}

		render.Map(os.Stdout, assembly, findings, mapSimulateOpen, simulateExtra)

		if mapCompose {
			render.Compose(os.Stdout, assembly)
		}

		return nil
	},
}

func collectSimulateFiles(assembly *model.ContextAssembly, target string) []model.ClaudeFile {
	targetSlash := filepath.ToSlash(target)
	var extra []model.ClaudeFile

	for _, f := range assembly.LazyFiles {
		switch f.LoadTiming {
		case model.LoadLazyDir:
			dir := filepath.Dir(f.Path)
			if strings.HasPrefix(target, dir+string(filepath.Separator)) || strings.HasPrefix(targetSlash, filepath.ToSlash(dir)+"/") {
				extra = append(extra, f)
			}
		case model.LoadLazyRule:
			rel, err := filepath.Rel(assembly.Workdir, target)
			if err == nil {
				relSlash := filepath.ToSlash(rel)
				if analyze.GlobMatchAny(f.PathGlobs, relSlash) {
					extra = append(extra, f)
				}
			}
		}
	}
	return extra
}
