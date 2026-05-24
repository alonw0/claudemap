package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alonw0/claudemap/model"
)

func TestDiscover_LoadOrder(t *testing.T) {
	// Create a temp dir tree: root/a/b/c/d (4 levels deep)
	root := t.TempDir()
	dirs := []string{root}
	cur := root
	for _, sub := range []string{"a", "b", "c", "d"} {
		cur = filepath.Join(cur, sub)
		if err := os.MkdirAll(cur, 0755); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, cur)
	}

	// Write a CLAUDE.md at each level
	for _, d := range dirs {
		content := "# " + filepath.Base(d) + " level\n- rule\n"
		if err := os.WriteFile(filepath.Join(d, "CLAUDE.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	workdir := cur // deepest: root/a/b/c/d
	assembly, err := Discover(workdir, Opts{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Find project-scope files and verify root comes before deeper files
	var projectFiles []model.ClaudeFile
	for _, f := range assembly.EagerFiles {
		if f.Scope == model.ScopeProjectRoot {
			projectFiles = append(projectFiles, f)
		}
	}

	if len(projectFiles) < 5 {
		t.Fatalf("expected at least 5 project files, got %d", len(projectFiles))
	}

	// Verify load order is ascending (root-first = lower order numbers first)
	for i := 1; i < len(projectFiles); i++ {
		if projectFiles[i].LoadOrder <= projectFiles[i-1].LoadOrder {
			t.Errorf("file[%d] LoadOrder=%d is not > file[%d] LoadOrder=%d",
				i, projectFiles[i].LoadOrder, i-1, projectFiles[i-1].LoadOrder)
		}
	}

	// Root's CLAUDE.md should appear before deeper ones
	rootFile := projectFiles[0]
	deepFile := projectFiles[len(projectFiles)-1]
	if rootFile.LoadOrder >= deepFile.LoadOrder {
		t.Errorf("root file (%s, order=%d) should come before deep file (%s, order=%d)",
			rootFile.Path, rootFile.LoadOrder, deepFile.Path, deepFile.LoadOrder)
	}
}

func TestDiscover_NoDuplicates(t *testing.T) {
	root := t.TempDir()
	// Create ~/.claude/CLAUDE.md equivalent under a subdir
	dot := filepath.Join(root, ".claude")
	if err := os.MkdirAll(dot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dot, "CLAUDE.md"), []byte("# user\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Also create a project CLAUDE.md
	subdir := filepath.Join(root, "project")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "CLAUDE.md"), []byte("# project\n"), 0644); err != nil {
		t.Fatal(err)
	}

	assembly, err := Discover(subdir, Opts{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Check for duplicate paths
	seen := map[string]int{}
	for _, f := range assembly.EagerFiles {
		seen[f.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("path %s appears %d times in eager files", path, count)
		}
	}
}

func TestDiscover_LazyDirFiles(t *testing.T) {
	root := t.TempDir()
	// Root CLAUDE.md
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# root\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Subdir CLAUDE.md (lazy)
	sub := filepath.Join(root, "subpkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "CLAUDE.md"), []byte("# subpkg\n"), 0644); err != nil {
		t.Fatal(err)
	}

	assembly, err := Discover(root, Opts{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Root CLAUDE.md should be eager
	foundEager := false
	for _, f := range assembly.EagerFiles {
		if filepath.Base(f.Path) == "CLAUDE.md" && filepath.Dir(f.Path) == root {
			foundEager = true
			if f.LoadTiming != model.LoadEager {
				t.Errorf("root CLAUDE.md should be eager, got %v", f.LoadTiming)
			}
		}
	}
	if !foundEager {
		t.Error("root CLAUDE.md not found in eager files")
	}

	// Sub CLAUDE.md should be lazy
	foundLazy := false
	for _, f := range assembly.LazyFiles {
		if filepath.Base(f.Path) == "CLAUDE.md" && filepath.Dir(f.Path) == sub {
			foundLazy = true
			if f.LoadTiming != model.LoadLazyDir {
				t.Errorf("sub CLAUDE.md should be LoadLazyDir, got %v", f.LoadTiming)
			}
		}
	}
	if !foundLazy {
		t.Error("subpkg CLAUDE.md not found in lazy files")
	}
}
