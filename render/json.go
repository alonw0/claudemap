package render

import (
	"encoding/json"
	"time"

	"github.com/alonw0/claudemap/model"
)

const Version = "0.1.0"

type jsonOutput struct {
	ClaudemapVersion string      `json:"claudemap_version"`
	Workdir          string      `json:"workdir"`
	Timestamp        string      `json:"timestamp"`
	Assembly         jsonAssembly `json:"assembly"`
	Findings         []jsonFinding `json:"findings"`
	Summary          jsonSummary `json:"summary"`
}

type jsonAssembly struct {
	EagerTokenTotal int         `json:"eager_token_total"`
	EagerLineTotal  int         `json:"eager_line_total"`
	EagerFiles      []jsonFile  `json:"eager_files"`
	LazyFiles       []jsonFile  `json:"lazy_files"`
}

type jsonFile struct {
	Path       string       `json:"path"`
	RelPath    string       `json:"rel_path"`
	Scope      string       `json:"scope"`
	LoadTiming string       `json:"load_timing"`
	LoadOrder  int          `json:"load_order"`
	LineCount  int          `json:"line_count"`
	Tokens     int          `json:"tokens"`
	Imports    []jsonImport `json:"imports"`
	PathGlobs  []string     `json:"path_globs"`
}

type jsonImport struct {
	Raw          string       `json:"raw"`
	Resolved     string       `json:"resolved,omitempty"`
	Depth        int          `json:"depth"`
	Exists       bool         `json:"exists"`
	IsCircular   bool         `json:"is_circular"`
	ExceedsDepth bool         `json:"exceeds_depth"`
	Children     []jsonImport `json:"children,omitempty"`
}

type jsonFinding struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
}

type jsonSummary struct {
	TotalFindings int `json:"total_findings"`
	Errors        int `json:"errors"`
	Warnings      int `json:"warnings"`
	Info          int `json:"info"`
}

func JSON(assembly *model.ContextAssembly, findings []model.Finding) ([]byte, error) {
	out := jsonOutput{
		ClaudemapVersion: Version,
		Workdir:          assembly.Workdir,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Assembly: jsonAssembly{
			EagerTokenTotal: assembly.EagerTokenTotal,
			EagerLineTotal:  assembly.EagerLineTotal,
			EagerFiles:      toJSONFiles(assembly.EagerFiles),
			LazyFiles:       toJSONFiles(assembly.LazyFiles),
		},
		Findings: toJSONFindings(findings),
		Summary:  toJSONSummary(findings),
	}
	if out.Assembly.EagerFiles == nil {
		out.Assembly.EagerFiles = []jsonFile{}
	}
	if out.Assembly.LazyFiles == nil {
		out.Assembly.LazyFiles = []jsonFile{}
	}
	if out.Findings == nil {
		out.Findings = []jsonFinding{}
	}
	return json.MarshalIndent(out, "", "  ")
}

func toJSONFiles(files []model.ClaudeFile) []jsonFile {
	out := make([]jsonFile, 0, len(files))
	for _, f := range files {
		jf := jsonFile{
			Path:      f.Path,
			RelPath:   f.RelPath,
			Scope:     f.Scope.JSONString(),
			LoadTiming: f.LoadTiming.String(),
			LoadOrder: f.LoadOrder,
			LineCount: f.LineCount,
			Tokens:    f.Tokens,
			Imports:   toJSONImports(f.Imports),
			PathGlobs: f.PathGlobs,
		}
		out = append(out, jf)
	}
	return out
}

func toJSONImports(refs []model.ImportRef) []jsonImport {
	out := make([]jsonImport, 0, len(refs))
	for _, r := range refs {
		ji := jsonImport{
			Raw:          r.Raw,
			Resolved:     r.Resolved,
			Depth:        r.Depth,
			Exists:       r.Exists,
			IsCircular:   r.IsCircular,
			ExceedsDepth: r.ExceedsDepth,
			Children:     toJSONImports(r.Children),
		}
		out = append(out, ji)
	}
	return out
}

func toJSONFindings(findings []model.Finding) []jsonFinding {
	out := make([]jsonFinding, 0, len(findings))
	for _, f := range findings {
		out = append(out, jsonFinding{
			ID:       f.ID,
			Code:     string(f.Code),
			Severity: string(f.Severity),
			FilePath: f.File.Path,
			Line:     f.Line,
			Message:  f.Message,
			Detail:   f.Detail,
		})
	}
	return out
}

func toJSONSummary(findings []model.Finding) jsonSummary {
	s := jsonSummary{TotalFindings: len(findings)}
	for _, f := range findings {
		switch f.Severity {
		case model.SeverityError:
			s.Errors++
		case model.SeverityWarning:
			s.Warnings++
		case model.SeverityInfo:
			s.Info++
		}
	}
	return s
}
