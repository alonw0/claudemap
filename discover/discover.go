package discover

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	gitignore "github.com/sabhiram/go-gitignore"
	"gopkg.in/yaml.v3"

	"github.com/alonw0/claudemap/analyze"
	"github.com/alonw0/claudemap/model"
)

var defaultIgnoreDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".next":        true,
	"__pycache__":  true,
	"target":       true,
}

type Opts struct {
	RespectGitignore bool
}

func Discover(workdir string, opts Opts) (*model.ContextAssembly, error) {
	workdir, err := filepath.Abs(workdir)
	if err != nil {
		return nil, err
	}

	assembly := &model.ContextAssembly{Workdir: workdir}
	orderSeq := 1

	// Managed policy
	managedPath := ManagedPolicyPath()
	if f, err := loadFile(managedPath, workdir); err == nil {
		f.Scope = model.ScopeManaged
		f.LoadTiming = model.LoadEager
		f.LoadOrder = 0
		assembly.EagerFiles = append(assembly.EagerFiles, f)
	}

	// User-level ~/.claude/CLAUDE.md
	home, _ := os.UserHomeDir()
	if home != "" {
		userMD := filepath.Join(home, ".claude", "CLAUDE.md")
		if f, err := loadFile(userMD, workdir); err == nil {
			f.Scope = model.ScopeUser
			f.LoadTiming = model.LoadEager
			f.LoadOrder = orderSeq
			orderSeq++
			assembly.EagerFiles = append(assembly.EagerFiles, f)
		}

		// User rules ~/.claude/rules/*.md
		userRulesDir := filepath.Join(home, ".claude", "rules")
		userRules, _ := collectRulesDir(userRulesDir, workdir, model.ScopeUserRule, &orderSeq)
		assembly.EagerFiles = append(assembly.EagerFiles, filterEager(userRules)...)
		assembly.LazyFiles = append(assembly.LazyFiles, filterLazy(userRules)...)
	}

	// Track seen paths to avoid duplicates (e.g. ~/.claude/CLAUDE.md found both
	// explicitly and during upward walk through home directory)
	seenPaths := map[string]bool{}
	for _, f := range assembly.EagerFiles {
		seenPaths[f.Path] = true
	}

	// Upward walk: collect directory groups from workdir → root, then reverse groups
	// so root-level files get lower load-order numbers. Within each group, preserve
	// collection order so CLAUDE.md < CLAUDE.local.md < .claude/CLAUDE.md < .claude/CLAUDE.local.md.
	type dirGroup struct {
		files []model.ClaudeFile
	}
	var dirGroups []dirGroup

	dir := workdir
	for {
		candidates := []struct {
			name  string
			scope model.Scope
		}{
			{"CLAUDE.md", model.ScopeProjectRoot},
			{"CLAUDE.local.md", model.ScopeProjectLocal},
			{filepath.Join(".claude", "CLAUDE.md"), model.ScopeProjectRoot},
			{filepath.Join(".claude", "CLAUDE.local.md"), model.ScopeProjectLocal},
		}
		var group dirGroup
		for _, c := range candidates {
			p := filepath.Join(dir, c.name)
			absP, _ := filepath.Abs(p)
			if seenPaths[absP] {
				continue
			}
			if f, err := loadFile(p, workdir); err == nil {
				f.Scope = c.scope
				f.LoadTiming = model.LoadEager
				seenPaths[absP] = true
				group.files = append(group.files, f)
			}
		}
		if len(group.files) > 0 {
			dirGroups = append(dirGroups, group)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Reverse groups only — within each group, order is preserved (CLAUDE.md before CLAUDE.local.md)
	for i, j := 0, len(dirGroups)-1; i < j; i, j = i+1, j-1 {
		dirGroups[i], dirGroups[j] = dirGroups[j], dirGroups[i]
	}
	for _, g := range dirGroups {
		for _, f := range g.files {
			f.LoadOrder = orderSeq
			orderSeq++
			assembly.EagerFiles = append(assembly.EagerFiles, f)
		}
	}

	// Project rules ./.claude/rules/*.md
	projectRulesDir := filepath.Join(workdir, ".claude", "rules")
	projectRules, _ := collectRulesDir(projectRulesDir, workdir, model.ScopeProjectRule, &orderSeq)
	assembly.EagerFiles = append(assembly.EagerFiles, filterEager(projectRules)...)
	assembly.LazyFiles = append(assembly.LazyFiles, filterLazy(projectRules)...)

	// Downward walk: subdirectories of workdir → lazy dir files
	lazyDirs, _ := walkSubdirs(workdir, opts)
	assembly.LazyFiles = append(assembly.LazyFiles, lazyDirs...)

	// Compute totals and build composed blocks
	for i := range assembly.EagerFiles {
		assembly.EagerTokenTotal += assembly.EagerFiles[i].Tokens
		assembly.EagerLineTotal += assembly.EagerFiles[i].LineCount
		assembly.ComposedBlocks = append(assembly.ComposedBlocks, model.ComposedBlock{
			Source:  assembly.EagerFiles[i],
			Content: assembly.EagerFiles[i].CleanContent,
			Tokens:  assembly.EagerFiles[i].Tokens,
		})
	}

	return assembly, nil
}

func loadFile(path, workdir string) (model.ClaudeFile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return model.ClaudeFile{}, err
	}

	f := model.ClaudeFile{Path: path}

	rel, err := filepath.Rel(workdir, path)
	if err == nil {
		f.RelPath = rel
	} else {
		f.RelPath = path
	}

	if info.Mode()&os.ModeSymlink != 0 {
		f.IsSymlink = true
		target, err := os.Readlink(path)
		if err == nil {
			f.SymlinkTarget = target
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return model.ClaudeFile{}, err
	}
	f.RawContent = string(raw)
	f.CleanContent = analyze.StripHTMLComments(f.RawContent)
	f.LineCount = strings.Count(f.RawContent, "\n")
	if len(f.RawContent) > 0 && f.RawContent[len(f.RawContent)-1] != '\n' {
		f.LineCount++
	}
	f.Tokens = analyze.EstimateTokens(f.RawContent)

	seen := map[string]bool{path: true}
	f.Imports = resolveImports(path, f.RawContent, 1, seen)

	return f, nil
}

type frontmatter struct {
	Paths []string `yaml:"paths"`
}

func parseFrontmatter(content string) ([]string, bool) {
	if !strings.HasPrefix(content, "---") {
		return nil, false
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return nil, false
	}
	block := content[3 : end+3]
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return nil, false
	}
	return fm.Paths, len(fm.Paths) > 0
}

func collectRulesDir(dir, workdir string, scope model.Scope, orderSeq *int) ([]model.ClaudeFile, error) {
	var files []model.ClaudeFile
	seenInodes := map[uint64]bool{}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Symlink cycle detection via inode
		info, err := os.Lstat(path)
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil
			}
			rInfo, err := os.Stat(resolved)
			if err != nil {
				return nil
			}
			stat, ok := rInfo.Sys().(*syscall.Stat_t)
			if ok {
				if seenInodes[stat.Ino] {
					// circular symlink — add a placeholder file with the flag
					cf := model.ClaudeFile{
						Path:               path,
						IsSymlink:          true,
						HasCircularSymlink: true,
						Scope:              scope,
						LoadTiming:         model.LoadEager,
					}
					if rel, err := filepath.Rel(workdir, path); err == nil {
						cf.RelPath = rel
					}
					files = append(files, cf)
					return nil
				}
				seenInodes[stat.Ino] = true
			}
		}

		f, err := loadFile(path, workdir)
		if err != nil {
			return nil
		}
		f.Scope = scope

		paths, hasGlobs := parseFrontmatter(f.RawContent)
		if hasGlobs {
			f.PathGlobs = paths
			f.LoadTiming = model.LoadLazyRule
		} else {
			f.LoadTiming = model.LoadEager
			f.LoadOrder = *orderSeq
			*orderSeq++
		}

		files = append(files, f)
		return nil
	})
	return files, err
}

func filterEager(files []model.ClaudeFile) []model.ClaudeFile {
	var out []model.ClaudeFile
	for _, f := range files {
		if f.LoadTiming == model.LoadEager {
			out = append(out, f)
		}
	}
	return out
}

func filterLazy(files []model.ClaudeFile) []model.ClaudeFile {
	var out []model.ClaudeFile
	for _, f := range files {
		if f.LoadTiming != model.LoadEager {
			out = append(out, f)
		}
	}
	return out
}

func walkSubdirs(workdir string, opts Opts) ([]model.ClaudeFile, error) {
	var files []model.ClaudeFile

	// Build gitignore matcher for workdir if requested
	var ignorer *gitignore.GitIgnore
	if opts.RespectGitignore {
		gitignorePath := filepath.Join(workdir, ".gitignore")
		if _, err := os.Stat(gitignorePath); err == nil {
			ignorer, _ = gitignore.CompileIgnoreFile(gitignorePath)
		}
	}

	err := filepath.WalkDir(workdir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if path == workdir {
			return nil
		}

		if d.IsDir() {
			name := d.Name()
			if defaultIgnoreDirs[name] {
				return filepath.SkipDir
			}
			// workdir's own .claude/ is handled by the upward walk and rules discovery;
			// skip it here to avoid double-counting workdir/.claude/CLAUDE.md as lazy.
			if name == ".claude" && filepath.Dir(path) == workdir {
				return filepath.SkipDir
			}
			if ignorer != nil {
				rel, _ := filepath.Rel(workdir, path)
				if ignorer.MatchesPath(rel) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		base := filepath.Base(path)
		if base != "CLAUDE.md" && base != "CLAUDE.local.md" {
			return nil
		}

		// Must not be in workdir itself (that's eager)
		if filepath.Dir(path) == workdir {
			return nil
		}

		f, err := loadFile(path, workdir)
		if err != nil {
			return nil
		}
		if base == "CLAUDE.local.md" {
			f.Scope = model.ScopeSubdirLocal
		} else {
			f.Scope = model.ScopeSubdirectory
		}
		f.LoadTiming = model.LoadLazyDir
		files = append(files, f)
		return nil
	})
	return files, err
}
