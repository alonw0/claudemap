package cmd_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// binaryPath builds the claudemap binary into a temp dir and returns its path.
func binaryPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(file))

	bin := filepath.Join(t.TempDir(), "claudemap")
	out, err := exec.Command("go", "build", "-o", bin, root).CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func testdata(path string) string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(file))
	return filepath.Join(root, "testdata", path)
}

// runCheck runs claudemap check in dir, returning stdout, stderr, and exit code.
func runCheck(t *testing.T, bin, dir string, args ...string) (stdout []byte, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"check"}, args...)...)
	cmd.Dir = dir
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return stdoutBuf.Bytes(), exit.ExitCode()
		}
		t.Fatalf("unexpected error running claudemap: %v", err)
	}
	return stdoutBuf.Bytes(), 0
}

func TestCheck_CleanProject_ExitZero(t *testing.T) {
	bin := binaryPath(t)
	_, code := runCheck(t, bin, testdata("clean"))
	if code != 0 {
		t.Errorf("expected exit 0 for clean project, got %d", code)
	}
}

func TestCheck_BrokenImports_ExitTwo(t *testing.T) {
	bin := binaryPath(t)
	_, code := runCheck(t, bin, testdata("broken-imports"))
	if code != 2 {
		t.Errorf("expected exit 2 for broken imports, got %d", code)
	}
}

func TestCheck_SimpleFixture_HasWarning(t *testing.T) {
	// testdata/simple has a W02 dead-rule warning → exit 2
	bin := binaryPath(t)
	_, code := runCheck(t, bin, testdata("simple"))
	if code != 2 {
		t.Errorf("expected exit 2 for simple fixture (has dead-rule warning), got %d", code)
	}
}

func TestCheck_JSON_ValidSchema(t *testing.T) {
	bin := binaryPath(t)
	out, _ := runCheck(t, bin, testdata("clean"), "--json")

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}

	for _, key := range []string{"claudemap_version", "workdir", "assembly", "findings", "summary"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing top-level JSON key %q", key)
		}
	}

	asm, ok := result["assembly"].(map[string]any)
	if !ok {
		t.Fatal("assembly field missing or wrong type")
	}
	if _, ok := asm["composed_blocks"]; !ok {
		t.Error("assembly.composed_blocks missing")
	}
	if _, ok := asm["eager_files"]; !ok {
		t.Error("assembly.eager_files missing")
	}
}

func TestCheck_JSON_BrokenImports_HasFindings(t *testing.T) {
	bin := binaryPath(t)
	out, code := runCheck(t, bin, testdata("broken-imports"), "--json")
	if code != 2 {
		t.Errorf("expected exit 2 for broken-imports, got %d", code)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}

	findings, ok := result["findings"].([]any)
	if !ok || len(findings) == 0 {
		t.Error("expected findings for broken-imports fixture, got none")
	}

	summary, ok := result["summary"].(map[string]any)
	if !ok {
		t.Fatal("summary missing")
	}
	if summary["errors"].(float64) == 0 {
		t.Error("expected at least 1 error in summary")
	}
}
