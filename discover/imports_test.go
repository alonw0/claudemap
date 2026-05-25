package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveImports_Basic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "rules.md")
	writeFile(t, target, "# rules\n")
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "@rules.md\n")

	refs := resolveImports(filepath.Join(dir, "CLAUDE.md"), "@rules.md\n", 0, map[string]bool{})
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if !refs[0].Exists {
		t.Errorf("ref should exist")
	}
	if refs[0].Raw != "rules.md" {
		t.Errorf("Raw = %q, want %q", refs[0].Raw, "rules.md")
	}
}

func TestResolveImports_MissingFile(t *testing.T) {
	dir := t.TempDir()
	refs := resolveImports(filepath.Join(dir, "CLAUDE.md"), "@missing.md\n", 0, map[string]bool{})
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].Exists {
		t.Errorf("ref to missing file should have Exists=false")
	}
}

func TestResolveImports_CircularDetected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "other.md")
	writeFile(t, target, "# other\n")

	seen := map[string]bool{target: true}
	refs := resolveImports(filepath.Join(dir, "CLAUDE.md"), "@other.md\n", 0, seen)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if !refs[0].IsCircular {
		t.Errorf("ref should be circular")
	}
}

func TestResolveImports_DepthLimit(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "deep.md")
	writeFile(t, target, "# deep\n")

	refs := resolveImports(filepath.Join(dir, "CLAUDE.md"), "@deep.md\n", 5, map[string]bool{})
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if !refs[0].ExceedsDepth {
		t.Errorf("ref at depth 5 should have ExceedsDepth=true")
	}
}

func TestResolveImports_SkipsBacktickInlineCode(t *testing.T) {
	// @path inside inline backticks is NOT a real import — it's prose/documentation.
	// This is the regression test for the scanner bug fixed in imports.go.
	dir := t.TempDir()
	content := "Use `@path` references to import files.\n"
	refs := resolveImports(filepath.Join(dir, "CLAUDE.md"), content, 0, map[string]bool{})
	if len(refs) != 0 {
		t.Errorf("expected 0 refs for @path inside backticks, got %d: %+v", len(refs), refs)
	}
}

func TestResolveImports_SkipsFencedCodeBlock(t *testing.T) {
	dir := t.TempDir()
	content := "Example:\n```\n@some-path.md\n```\nEnd.\n"
	refs := resolveImports(filepath.Join(dir, "CLAUDE.md"), content, 0, map[string]bool{})
	if len(refs) != 0 {
		t.Errorf("expected 0 refs inside fenced code block, got %d", len(refs))
	}
}

func TestResolveImports_RealImportAfterBacktickProse(t *testing.T) {
	// A real import on one line, backtick prose on another — only the real one should fire.
	dir := t.TempDir()
	target := filepath.Join(dir, "rules.md")
	writeFile(t, target, "# rules\n")
	content := "Use `@path` syntax like this:\n@rules.md\n"
	refs := resolveImports(filepath.Join(dir, "CLAUDE.md"), content, 0, map[string]bool{})
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref (real import only), got %d: %+v", len(refs), refs)
	}
	if refs[0].Raw != "rules.md" {
		t.Errorf("Raw = %q, want %q", refs[0].Raw, "rules.md")
	}
}

func TestIsInsideBackticks(t *testing.T) {
	cases := []struct {
		line string
		pos  int
		want bool
	}{
		{"`@path`", 1, true},         // inside
		{"@path", 0, false},           // no backticks
		{"see `@path` for docs", 5, true},  // inside mid-line
		{"`code` and @real", 11, false},    // after closing backtick
		{"no backtick @real here", 14, false}, // no backtick, real import
	}
	for _, c := range cases {
		got := isInsideBackticks(c.line, c.pos)
		if got != c.want {
			t.Errorf("isInsideBackticks(%q, %d) = %v, want %v", c.line, c.pos, got, c.want)
		}
	}
}
