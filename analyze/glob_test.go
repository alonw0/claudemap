package analyze

import "testing"

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.ts", "src/api/foo.ts", true},
		{"**/*.ts", "src/api/foo.js", false},
		{"**/*.ts", "foo.ts", true},
		{"**/*.test.ts", "src/api/foo.test.ts", true},
		{"**/*.test.ts", "src/api/foo.ts", false},
		{"src/**/*.ts", "src/api/foo.ts", true},
		{"src/**/*.ts", "lib/api/foo.ts", false},
		{"*.md", "README.md", true},
		{"*.md", "dir/README.md", false},
		{"**", "anything/goes/here", true},
		{"**/CLAUDE.md", "foo/bar/CLAUDE.md", true},
		{"**/CLAUDE.md", "CLAUDE.md", true},
		{"src/api/*.ts", "src/api/foo.ts", true},
		{"src/api/*.ts", "src/api/sub/foo.ts", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"|"+tt.path, func(t *testing.T) {
			got := GlobMatch(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("GlobMatch(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
