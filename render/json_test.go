package render

import (
	"encoding/json"
	"testing"

	"github.com/alonw0/claudemap/model"
)

func TestJSON_Schema(t *testing.T) {
	assembly := &model.ContextAssembly{
		Workdir:         "/tmp/project",
		EagerTokenTotal: 42,
		EagerLineTotal:  10,
		EagerFiles: []model.ClaudeFile{
			{
				Path:       "/home/.claude/CLAUDE.md",
				RelPath:    "~/.claude/CLAUDE.md",
				Scope:      model.ScopeUser,
				LoadTiming: model.LoadEager,
				LoadOrder:  1,
				LineCount:  3,
				Tokens:     10,
			},
			{
				Path:       "/tmp/project/CLAUDE.md",
				RelPath:    "CLAUDE.md",
				Scope:      model.ScopeProjectRoot,
				LoadTiming: model.LoadEager,
				LoadOrder:  2,
				LineCount:  7,
				Tokens:     32,
			},
		},
		ComposedBlocks: []model.ComposedBlock{
			{
				Source: model.ClaudeFile{
					Path:      "/home/.claude/CLAUDE.md",
					Scope:     model.ScopeUser,
					LoadOrder: 1,
				},
				Content: "user rule content",
				Tokens:  10,
			},
			{
				Source: model.ClaudeFile{
					Path:      "/tmp/project/CLAUDE.md",
					Scope:     model.ScopeProjectRoot,
					LoadOrder: 2,
				},
				Content: "project rule content",
				Tokens:  32,
			},
		},
	}

	findings := []model.Finding{
		{
			ID:       "W-size-001",
			Code:     model.CodeSizeViolation,
			Severity: model.SeverityWarning,
			File:     assembly.EagerFiles[1],
			Line:     1,
			Message:  "file is large",
		},
	}

	raw, err := JSON(assembly, findings)
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Top-level keys
	for _, key := range []string{"claudemap_version", "workdir", "timestamp", "assembly", "findings", "summary"} {
		if _, ok := out[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}

	asm := out["assembly"].(map[string]any)

	// composed_blocks present and correct count
	blocks, ok := asm["composed_blocks"].([]any)
	if !ok {
		t.Fatal("assembly.composed_blocks missing or wrong type")
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 composed_blocks, got %d", len(blocks))
	}

	// First block fields
	b0 := blocks[0].(map[string]any)
	if b0["source_file"] != "/home/.claude/CLAUDE.md" {
		t.Errorf("block[0].source_file = %v", b0["source_file"])
	}
	if b0["scope"] != "user" {
		t.Errorf("block[0].scope = %v", b0["scope"])
	}
	if b0["load_order"].(float64) != 1 {
		t.Errorf("block[0].load_order = %v", b0["load_order"])
	}
	if b0["content"] != "user rule content" {
		t.Errorf("block[0].content = %v", b0["content"])
	}

	// findings present
	fList, ok := out["findings"].([]any)
	if !ok || len(fList) != 1 {
		t.Fatalf("expected 1 finding, got %v", out["findings"])
	}
	f0 := fList[0].(map[string]any)
	if f0["severity"] != "warning" {
		t.Errorf("finding severity = %v", f0["severity"])
	}

	// summary counts
	summary := out["summary"].(map[string]any)
	if summary["total_findings"].(float64) != 1 {
		t.Errorf("summary.total_findings = %v", summary["total_findings"])
	}
	if summary["warnings"].(float64) != 1 {
		t.Errorf("summary.warnings = %v", summary["warnings"])
	}
}

func TestJSON_EmptyAssembly(t *testing.T) {
	assembly := &model.ContextAssembly{Workdir: "/tmp"}
	raw, err := JSON(assembly, nil)
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}

	asm := out["assembly"].(map[string]any)

	// Empty slices should serialize as [] not null
	for _, key := range []string{"eager_files", "lazy_files"} {
		arr, ok := asm[key].([]any)
		if !ok {
			t.Errorf("assembly.%s should be [], got %T", key, asm[key])
			continue
		}
		if len(arr) != 0 {
			t.Errorf("assembly.%s should be empty, got %d items", key, len(arr))
		}
	}

	findings, ok := out["findings"].([]any)
	if !ok {
		t.Errorf("findings should be [], got %T", out["findings"])
	} else if len(findings) != 0 {
		t.Errorf("findings should be empty, got %d items", len(findings))
	}
}
