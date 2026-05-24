package model

type ClaudeFile struct {
	Path          string
	RelPath       string
	Scope         Scope
	LoadTiming    LoadTiming
	LoadOrder     int
	LineCount     int
	RawContent    string
	CleanContent  string
	Tokens        int
	Imports       []ImportRef
	PathGlobs     []string
	IsSymlink     bool
	SymlinkTarget string
	HasCircularSymlink bool
}

type Scope int

const (
	ScopeManaged     Scope = iota
	ScopeUser
	ScopeUserRule
	ScopeProjectRoot
	ScopeProjectLocal
	ScopeProjectRule
	ScopeSubdirectory
	ScopeSubdirLocal
)

func (s Scope) String() string {
	switch s {
	case ScopeManaged:
		return "managed"
	case ScopeUser:
		return "user"
	case ScopeUserRule:
		return "user-rule"
	case ScopeProjectRoot:
		return "project"
	case ScopeProjectLocal:
		return "local"
	case ScopeProjectRule:
		return "rule"
	case ScopeSubdirectory:
		return "subdir"
	case ScopeSubdirLocal:
		return "subdir-local"
	default:
		return "unknown"
	}
}

func (s Scope) JSONString() string {
	switch s {
	case ScopeManaged:
		return "managed"
	case ScopeUser:
		return "user"
	case ScopeUserRule:
		return "user_rule"
	case ScopeProjectRoot:
		return "project_root"
	case ScopeProjectLocal:
		return "project_local"
	case ScopeProjectRule:
		return "project_rule"
	case ScopeSubdirectory:
		return "subdirectory"
	case ScopeSubdirLocal:
		return "subdir_local"
	default:
		return "unknown"
	}
}

type LoadTiming int

const (
	LoadEager    LoadTiming = iota
	LoadLazyDir
	LoadLazyRule
)

func (l LoadTiming) String() string {
	switch l {
	case LoadEager:
		return "eager"
	case LoadLazyDir:
		return "lazy_dir"
	case LoadLazyRule:
		return "lazy_rule"
	default:
		return "unknown"
	}
}

type ImportRef struct {
	Raw          string
	Resolved     string
	Depth        int
	Exists       bool
	IsCircular   bool
	ExceedsDepth bool
	Children     []ImportRef
	Line         int
}

type ContextAssembly struct {
	Workdir         string
	EagerFiles      []ClaudeFile
	LazyFiles       []ClaudeFile
	EagerTokenTotal int
	EagerLineTotal  int
	ComposedBlocks  []ComposedBlock
}

type ComposedBlock struct {
	Source  ClaudeFile
	Content string
	Tokens  int
}

type Finding struct {
	ID       string
	Code     FindingCode
	Severity Severity
	File     ClaudeFile
	Line     int
	Message  string
	Detail   string
}

type FindingCode string

const (
	CodeBrokenImport   FindingCode = "broken-import"
	CodeImportDepth    FindingCode = "import-depth"
	CodeSizeViolation  FindingCode = "size-violation"
	CodeDeadRule       FindingCode = "dead-rule"
	CodeCircularImport FindingCode = "circular-import"
	CodeCircularSymlink FindingCode = "circular-symlink"
	CodeSizeApproaching FindingCode = "size-approaching"
	CodePostCompaction  FindingCode = "post-compaction-risk"
	CodeDeadExclude    FindingCode = "dead-exclude"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)
