package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alonw0/claudemap/analyze"
	"github.com/alonw0/claudemap/discover"
	"github.com/alonw0/claudemap/model"
	"github.com/alonw0/claudemap/render"
)

var (
	checkJSON       bool
	checkQuiet      bool
	checkReport     string
	checkOutput     string
	checkWorkdir    string
	checkNoGitignore bool
)

func init() {
	checkCmd.Flags().BoolVar(&checkJSON, "json", false, "Machine-readable JSON output")
	checkCmd.Flags().BoolVar(&checkQuiet, "quiet", false, "No stdout output; exit code only")
	checkCmd.Flags().StringVar(&checkReport, "report", "", "Report format: html")
	checkCmd.Flags().StringVar(&checkOutput, "output", "claudemap-report.html", "Output path for --report html")
	checkCmd.Flags().StringVar(&checkWorkdir, "workdir", "", "Run against a different directory (default: current directory)")
	checkCmd.Flags().BoolVar(&checkNoGitignore, "no-gitignore", false, "Ignore .gitignore files during traversal")
	rootCmd.AddCommand(checkCmd)
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run all issue detectors",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		workdir := checkWorkdir
		if workdir == "" {
			var err error
			workdir, err = os.Getwd()
			if err != nil {
				return err
			}
		}

		opts := discover.Opts{RespectGitignore: !checkNoGitignore}
		assembly, err := discover.Discover(workdir, opts)
		if err != nil {
			return fmt.Errorf("discovery failed: %w", err)
		}

		findings := analyze.RunChecks(assembly, workdir)

		exitCode := exitCodeFor(findings)

		if checkQuiet {
			os.Exit(exitCode)
		}

		if checkReport == "html" {
			out := checkOutput
			if !filepath.IsAbs(out) {
				wd, _ := os.Getwd()
				out = filepath.Join(wd, out)
			}
			if err := render.HTML(assembly, findings, out); err != nil {
				return err
			}
			os.Exit(exitCode)
		}

		if checkJSON {
			data, err := render.JSON(assembly, findings)
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			os.Exit(exitCode)
		}

		render.Check(os.Stdout, findings)
		os.Exit(exitCode)
		return nil
	},
}

func exitCodeFor(findings []model.Finding) int {
	hasError := false
	hasWarn := false
	hasInfo := false
	for _, f := range findings {
		switch f.Severity {
		case model.SeverityError:
			hasError = true
		case model.SeverityWarning:
			hasWarn = true
		case model.SeverityInfo:
			hasInfo = true
		}
	}
	if hasError || hasWarn {
		return 2
	}
	if hasInfo {
		return 1
	}
	return 0
}
