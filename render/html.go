package render

import (
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alonw0/claudemap/model"
)

type htmlData struct {
	Version         string
	Workdir         string
	Timestamp       string
	Assembly        *model.ContextAssembly
	Findings        []model.Finding
	EagerTokenTotal int
	EagerLineTotal  int
	ErrCount        int
	WarnCount       int
	InfoCount       int
	BarSegments     []barSegment
	FindingsByFile  map[string][]model.Finding
	Home            string
}

type barSegment struct {
	Percent float64
	Color   string
	Label   string
	Tokens  int
}

var segColors = []string{
	"#e8963a", "#5b9cf6", "#56cf8e", "#c084fc", "#f472b6",
	"#38bdf8", "#fb923c", "#a3e635", "#e879f9", "#34d399",
}

func HTML(assembly *model.ContextAssembly, findings []model.Finding, outputPath string) error {
	home, _ := os.UserHomeDir()

	errs, warns, infos := countSeverities(findings)

	// Build bar segments
	var segments []barSegment
	for i, f := range assembly.EagerFiles {
		if assembly.EagerTokenTotal == 0 {
			break
		}
		pct := float64(f.Tokens) / float64(assembly.EagerTokenTotal) * 100
		color := segColors[i%len(segColors)]
		label := f.RelPath
		if label == "" || strings.HasPrefix(label, "..") {
			label = friendlyHTMLPath(f.Path, home)
		}
		segments = append(segments, barSegment{
			Percent: pct,
			Color:   color,
			Label:   label,
			Tokens:  f.Tokens,
		})
	}

	// Index findings by file path
	findingsByFile := map[string][]model.Finding{}
	for _, f := range findings {
		findingsByFile[f.File.Path] = append(findingsByFile[f.File.Path], f)
	}

	data := &htmlData{
		Version:         Version,
		Workdir:         assembly.Workdir,
		Timestamp:       time.Now().Format("2006-01-02 15:04:05"),
		Assembly:        assembly,
		Findings:        findings,
		EagerTokenTotal: assembly.EagerTokenTotal,
		EagerLineTotal:  assembly.EagerLineTotal,
		ErrCount:        errs,
		WarnCount:       warns,
		InfoCount:       infos,
		BarSegments:     segments,
		FindingsByFile:  findingsByFile,
		Home:            home,
	}

	funcMap := template.FuncMap{
		"friendlyPath": func(p string) string { return friendlyHTMLPath(p, home) },
		"scopeClass":   scopeClass,
		"scopeLabel":   func(s model.Scope) string { return s.String() },
		"sevClass":     sevClass,
		"sevLabel":     sevLabel,
		"joinGlobs":    func(g []string) string { return strings.Join(g, ", ") },
		"hasFindings": func(path string, fbf map[string][]model.Finding) bool {
			return len(fbf[path]) > 0
		},
		"fileFindings": func(path string, fbf map[string][]model.Finding) []model.Finding {
			return fbf[path]
		},
		"timingLabel": func(f model.ClaudeFile) string {
			switch f.LoadTiming {
			case model.LoadLazyDir:
				return "on dir open"
			case model.LoadLazyRule:
				return strings.Join(f.PathGlobs, ", ")
			default:
				return "eager"
			}
		},
		"orderLabel": func(f model.ClaudeFile) string {
			if f.LoadOrder > 0 {
				return fmt.Sprintf("#%d", f.LoadOrder)
			}
			switch f.LoadTiming {
			case model.LoadLazyDir:
				return "dir"
			case model.LoadLazyRule:
				return "rule"
			default:
				return "#0"
			}
		},
		"loop": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
		"isLazy": func(f model.ClaudeFile) bool {
			return f.LoadTiming != model.LoadEager
		},
		"isDeadRule": func(f model.ClaudeFile, fbf map[string][]model.Finding) bool {
			for _, fi := range fbf[f.Path] {
				if fi.Code == model.CodeDeadRule {
					return true
				}
			}
			return false
		},
		"fmtTokens": func(n int) string { return fmt.Sprintf("~%d", n) },
		"add":        func(a, b int) int { return a + b },
		"htmlEscape": func(s string) template.HTML { return template.HTML(template.HTMLEscapeString(s)) },
		"neq":        func(a, b int) bool { return a != b },
		"maxf":       func(a, b float64) float64 { if a > b { return a }; return b },
		"isEager":    func(f model.ClaudeFile) bool { return f.LoadTiming == model.LoadEager },
		"lineWarning": func(f model.ClaudeFile) bool { return f.LineCount > 200 },
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("html template parse: %w", err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outputPath, err)
	}
	defer out.Close()

	if err := tmpl.Execute(out, data); err != nil {
		return fmt.Errorf("html template execute: %w", err)
	}

	abs, _ := filepath.Abs(outputPath)
	fmt.Fprintf(os.Stderr, "report written → %s\n", abs)
	openInBrowser(abs)
	return nil
}

func openInBrowser(path string) {
	url := "file://" + path
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func friendlyHTMLPath(p, home string) string {
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func scopeClass(s model.Scope) string {
	switch s {
	case model.ScopeManaged:
		return "scope-managed"
	case model.ScopeUser, model.ScopeUserRule:
		return "scope-user"
	case model.ScopeProjectRoot:
		return "scope-project"
	case model.ScopeProjectLocal:
		return "scope-local"
	case model.ScopeProjectRule:
		return "scope-rule"
	default:
		return "scope-lazy"
	}
}

func sevClass(s model.Severity) string {
	switch s {
	case model.SeverityError:
		return "sev-e"
	case model.SeverityWarning:
		return "sev-w"
	default:
		return "sev-i"
	}
}

func sevLabel(s model.Severity) string {
	switch s {
	case model.SeverityError:
		return "ERR"
	case model.SeverityWarning:
		return "WRN"
	default:
		return "INF"
	}
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>claudemap — context report</title>
<style>
  @import url('https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap');
  :root{--bg:#0a0a0b;--bg2:#111113;--bg3:#18181c;--bg4:#212126;--bg5:#2a2a30;--border:#252529;--border2:#333338;--text:#e2e2e6;--text2:#9898a6;--text3:#55555f;--accent:#f59e0b;--accent2:#451a03;--accent3:#78350f;--green:#22c55e;--green2:#052e16;--green3:#166534;--yellow:#eab308;--yellow2:#3a2000;--yellow3:#713f00;--red:#ef4444;--red2:#2d0c0c;--red3:#7f1d1d;--purple:#a78bfa;--purple2:#1e1030;--dim:#44444e;--mono:'JetBrains Mono',monospace;--sans:'Plus Jakarta Sans',system-ui,sans-serif;--radius:5px;--radius2:8px;--shadow:0 1px 3px rgba(0,0,0,.6),0 6px 20px rgba(0,0,0,.4);--shadow2:0 1px 2px rgba(0,0,0,.5)}
  *{box-sizing:border-box;margin:0;padding:0}
  body{background:var(--bg);color:var(--text);font-family:var(--sans);font-size:15px;line-height:1.6;min-height:100vh}
  .shell{display:grid;grid-template-columns:256px 1fr;grid-template-rows:48px 1fr;min-height:100vh}
  .topbar{grid-column:1/-1;background:var(--bg2);border-bottom:1px solid var(--border);display:flex;align-items:center;padding:0 18px;gap:14px}
  .topbar-logo{font-family:var(--mono);font-weight:600;font-size:14px;color:var(--accent);letter-spacing:-.5px;white-space:nowrap}
  .topbar-logo span{color:var(--text3);font-weight:400}
  .topbar-path{font-family:var(--mono);font-size:11px;color:var(--text3);background:var(--bg3);border:1px solid var(--border);border-radius:var(--radius);padding:3px 10px;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .topbar-meta{display:flex;gap:6px;align-items:center}
  .badge{font-family:var(--mono);font-size:10px;font-weight:600;padding:2px 7px;border-radius:3px;letter-spacing:.3px;white-space:nowrap}
  .badge-err{background:var(--red2);color:var(--red);border:1px solid var(--red3)}
  .badge-warn{background:var(--yellow2);color:var(--yellow);border:1px solid var(--yellow3)}
  .badge-info{background:var(--bg4);color:var(--text2);border:1px solid var(--border2)}
  .badge-ok{background:var(--green2);color:var(--green);border:1px solid var(--green3)}
  .sidebar{background:var(--bg2);border-right:1px solid var(--border);overflow-y:auto;padding:10px 0 20px}
  .sidebar-section{margin-bottom:6px}
  .sidebar-label{font-family:var(--mono);font-size:9px;font-weight:600;letter-spacing:1.5px;text-transform:uppercase;color:var(--text3);padding:10px 14px 4px}
  .sidebar-item{display:flex;align-items:center;gap:7px;padding:5px 14px;cursor:pointer;border-left:2px solid transparent;transition:background .08s;user-select:none}
  .sidebar-item:hover{background:var(--bg3)}
  .sidebar-item.active{background:var(--bg3);border-left-color:var(--accent)}
  .sidebar-item .si-icon{font-size:10px;width:12px;text-align:center;flex-shrink:0;color:var(--text3)}
  .sidebar-item.active .si-icon{color:var(--accent)}
  .sidebar-item .si-label{font-family:var(--mono);font-size:12px;color:var(--text2);flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .sidebar-item.active .si-label{color:var(--text)}
  .sidebar-item .si-badge{font-family:var(--mono);font-size:10px;padding:1px 5px;border-radius:2px;flex-shrink:0}
  .si-badge-e{background:var(--red2);color:var(--red)}
  .si-badge-w{background:var(--yellow2);color:var(--yellow)}
  .si-badge-i{background:var(--bg4);color:var(--text3)}
  .si-badge-ok{background:var(--bg4);color:var(--text3)}
  .main{overflow-y:auto;padding:18px;display:flex;flex-direction:column;gap:14px;background:var(--bg)}
  .card{background:var(--bg2);border:1px solid var(--border);border-radius:var(--radius2);overflow:hidden;box-shadow:var(--shadow)}
  .card-head{display:flex;align-items:center;gap:10px;padding:10px 16px;border-bottom:1px solid var(--border);background:var(--bg3)}
  .card-title{font-family:var(--mono);font-size:10px;font-weight:600;letter-spacing:1.2px;text-transform:uppercase;color:var(--text);flex:1}
  .card-sub{font-family:var(--mono);font-size:10px;color:var(--text3)}
  .summary-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:1px;background:var(--border)}
  .summary-cell{background:var(--bg2);padding:20px 20px 16px;display:flex;flex-direction:column;gap:6px;transition:background .1s}
  .summary-cell:hover{background:var(--bg3)}
  .sc-value{font-family:var(--mono);font-size:34px;font-weight:600;line-height:1;letter-spacing:-1px}
  .sc-label{font-size:10px;color:var(--text3);font-family:var(--mono);letter-spacing:.6px;text-transform:uppercase}
  .sc-sub{font-size:10px;color:var(--text3);font-family:var(--mono)}
  .sc-err{color:var(--red)}.sc-warn{color:var(--yellow)}.sc-accent{color:var(--accent)}.sc-green{color:var(--green)}
  .token-bar-wrap{padding:14px 16px;display:flex;flex-direction:column;gap:12px;border-top:1px solid var(--border)}
  .token-bar{display:flex;height:10px;border-radius:2px;overflow:hidden;background:var(--bg4);gap:1px}
  .tb-seg{height:100%;transition:opacity .1s}.tb-seg:hover{opacity:.7}
  .token-legend{display:flex;flex-wrap:wrap;gap:8px 18px}
  .tl-item{display:flex;align-items:center;gap:5px;font-family:var(--mono);font-size:11px;color:var(--text2)}
  .tl-dot{width:7px;height:7px;border-radius:1px;flex-shrink:0}
  .tl-tokens{color:var(--text3);margin-left:1px}
  .tree{padding:4px 0}
  .tree-row-inner{display:flex;align-items:center;gap:8px;flex:1;padding:5px 14px;border-radius:var(--radius);margin:1px 6px;transition:background .08s}
  .tree-row-inner:hover{background:var(--bg3)}
  .tree-order{font-family:var(--mono);font-size:10px;color:var(--text3);background:var(--bg4);border:1px solid var(--border);border-radius:3px;padding:1px 5px;min-width:30px;text-align:center;flex-shrink:0}
  .tree-icon{font-size:13px;flex-shrink:0}
  .tree-name{font-family:var(--mono);font-size:13px;color:var(--text);flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .tree-scope{font-family:var(--mono);font-size:10px;padding:1px 5px;border-radius:2px;flex-shrink:0;font-weight:500;letter-spacing:.2px}
  .scope-managed{background:var(--purple2);color:var(--purple)}
  .scope-user{background:var(--bg4);color:var(--text3)}
  .scope-project{background:var(--accent2);color:var(--accent);border:1px solid var(--accent3)}
  .scope-local{background:var(--bg4);color:var(--text3)}
  .scope-lazy{background:var(--bg4);color:var(--dim)}
  .scope-rule{background:var(--green2);color:var(--green);border:1px solid var(--green3)}
  .tree-timing{font-family:var(--mono);font-size:10px;color:var(--text3);flex-shrink:0;max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .tree-lines{font-family:var(--mono);font-size:12px;color:var(--text3);flex-shrink:0;min-width:48px;text-align:right}
  .tree-tokens{font-family:var(--mono);font-size:12px;color:var(--text3);flex-shrink:0;min-width:60px;text-align:right}
  .tree-findings{display:flex;gap:3px;flex-shrink:0;min-width:20px}
  .tf-dot{width:5px;height:5px;border-radius:50%;margin-top:1px}
  .tf-dot-e{background:var(--red)}.tf-dot-w{background:var(--yellow)}.tf-dot-i{background:var(--text3)}
  .tree-import-row{display:flex;align-items:center;gap:8px;padding:2px 14px 2px 46px;margin:0 6px}
  .import-arrow{font-family:var(--mono);font-size:10px;color:var(--text3);flex-shrink:0}
  .import-name{font-family:var(--mono);font-size:11px;color:var(--text2);flex:1}
  .import-depth{font-family:var(--mono);font-size:10px;color:var(--text3);background:var(--bg4);border:1px solid var(--border);border-radius:2px;padding:0 4px}
  .import-error{color:var(--red)}.import-ok{color:var(--text3)}
  .tree-section-divider{display:flex;align-items:center;gap:10px;padding:10px 14px 6px;margin-top:2px}
  .tsd-label{font-family:var(--mono);font-size:9px;font-weight:600;letter-spacing:1px;text-transform:uppercase;color:var(--text3);white-space:nowrap}
  .tsd-line{height:1px;background:var(--border);flex:1}
  .findings-list{display:flex;flex-direction:column}
  .finding-item{display:flex;gap:14px;padding:14px 16px;border-bottom:1px solid var(--border);transition:background .1s}
  .finding-item:last-child{border-bottom:none}
  .finding-item:hover{background:var(--bg3)}
  .finding-sev{width:34px;flex-shrink:0;display:flex;align-items:flex-start;padding-top:1px}
  .sev-icon{font-family:var(--mono);font-size:10px;font-weight:600;padding:2px 5px;border-radius:2px;letter-spacing:.4px}
  .sev-e{background:var(--red2);color:var(--red);border:1px solid var(--red3)}
  .sev-w{background:var(--yellow2);color:var(--yellow);border:1px solid var(--yellow3)}
  .sev-i{background:var(--bg4);color:var(--text3);border:1px solid var(--border2)}
  .finding-body{flex:1;min-width:0}
  .finding-code{font-family:var(--mono);font-size:10px;color:var(--text3);margin-bottom:4px;letter-spacing:.4px;text-transform:uppercase}
  .finding-msg{font-size:14px;color:var(--text);margin-bottom:4px;line-height:1.5;font-weight:500}
  .finding-file{font-family:var(--mono);font-size:12px;color:var(--accent);margin-bottom:4px}
  .finding-detail{font-size:12px;color:var(--text2);line-height:1.55;background:var(--bg3);border:1px solid var(--border);border-left:2px solid var(--border2);border-radius:var(--radius);padding:8px 12px;margin-top:6px;font-family:var(--mono)}
  .compose-view-toggle{display:flex;background:var(--bg4);border:1px solid var(--border);border-radius:var(--radius);padding:2px;gap:1px;flex-shrink:0}
  .cvt-btn{background:none;border:none;cursor:pointer;font-family:var(--mono);font-size:11px;color:var(--text3);padding:3px 10px;border-radius:3px;transition:background .1s,color .1s}
  .cvt-btn:hover{color:var(--text2)}
  .cvt-btn.cvt-active{background:var(--bg2);color:var(--text);box-shadow:var(--shadow2)}
  .compose-copy-btn{background:none;border:1px solid var(--border);cursor:pointer;font-family:var(--mono);font-size:11px;color:var(--text3);padding:4px 10px;border-radius:var(--radius);transition:background .1s,color .1s,border-color .1s;flex-shrink:0;display:flex;align-items:center;gap:5px}
  .compose-copy-btn:hover{background:var(--bg3);color:var(--text2);border-color:var(--border2)}
  .compose-copy-btn.copied{color:var(--green);border-color:var(--green3)}
  .compose-combined-wrap{overflow-y:auto}
  .compose-combined-sep{display:flex;align-items:center;gap:10px;padding:10px 16px 8px;border-top:1px solid var(--border);border-bottom:1px solid var(--border);background:var(--bg3);position:sticky;top:0;z-index:1}
  .compose-combined-sep:first-child{border-top:none}
  .ccs-order{font-family:var(--mono);font-size:10px;color:var(--text3);background:var(--bg4);border:1px solid var(--border);border-radius:2px;padding:1px 5px}
  .ccs-file{font-family:var(--mono);font-size:12px;color:var(--text);flex:1}
  .ccs-scope{font-family:var(--mono);font-size:10px;color:var(--text3)}
  .ccs-tokens{font-family:var(--mono);font-size:11px;color:var(--text3)}
  .compose-combined-pre{font-family:var(--mono);font-size:13px;color:var(--text2);line-height:1.65;padding:16px;white-space:pre-wrap;word-break:break-word;border-bottom:1px solid var(--border)}
  .compose-combined-pre:last-of-type{border-bottom:none}
  .ln{display:block}
  .ln-num{display:inline-block;text-align:right;color:var(--text3);opacity:.45;user-select:none;-webkit-user-select:none;padding-right:14px;border-right:1px solid var(--border);margin-right:14px;font-size:.85em;min-width:var(--ln-w,3ch)}
  .ln-text{color:inherit}
  .compose-combined-lazy-note{padding:12px 16px;font-family:var(--mono);font-size:11px;color:var(--text3);border-top:1px solid var(--border);background:var(--bg3)}
  .compose-section-header{display:flex;align-items:center;gap:10px;padding:10px 16px 6px;font-family:var(--mono);font-size:9px;letter-spacing:1px;text-transform:uppercase;color:var(--text3);border-top:1px solid var(--border)}
  .compose-block{border-bottom:1px solid var(--border)}
  .compose-block:last-child{border-bottom:none}
  .compose-header{display:flex;align-items:center;gap:10px;padding:10px 16px;background:var(--bg3);border-bottom:1px solid var(--border);cursor:pointer;user-select:none;transition:background .08s}
  .compose-header:hover{background:var(--bg4)}
  .ch-order{font-family:var(--mono);font-size:10px;color:var(--text3);background:var(--bg4);border:1px solid var(--border);border-radius:2px;padding:1px 5px;min-width:24px;text-align:center}
  .ch-file{font-family:var(--mono);font-size:13px;color:var(--text);flex:1}
  .ch-tokens{font-family:var(--mono);font-size:11px;color:var(--text3)}
  .ch-toggle{font-size:9px;color:var(--text3);transition:transform .15s}
  .compose-content{padding:0;display:none}
  .compose-content.open{display:block}
  .compose-content pre{font-family:var(--mono);font-size:13px;color:var(--text2);line-height:1.65;padding:16px;overflow-x:auto;white-space:pre-wrap;word-break:break-word;border-top:1px solid var(--border)}
  .tabs{display:flex;gap:0;border-bottom:1px solid var(--border);background:var(--bg3);padding:0 14px}
  .tab{font-family:var(--mono);font-size:12px;color:var(--text3);padding:9px 12px;cursor:pointer;border-bottom:2px solid transparent;margin-bottom:-1px;transition:color .1s;user-select:none}
  .tab:hover{color:var(--text2)}
  .tab.active{color:var(--accent);border-bottom-color:var(--accent)}
  ::-webkit-scrollbar{width:5px;height:5px}
  ::-webkit-scrollbar-track{background:var(--bg)}
  ::-webkit-scrollbar-thumb{background:var(--border2);border-radius:3px}
  /* CONTEXT MAP VIEW */
  .cmap-treemap{display:flex;gap:6px;padding:20px;min-height:220px;align-items:stretch}
  .cmap-tm-cell{border-radius:8px;border:1px solid;padding:14px;display:flex;flex-direction:column;justify-content:flex-end;cursor:pointer;overflow:hidden;transition:filter .12s,transform .12s;min-width:64px;position:relative}
  .cmap-tm-cell:hover{filter:brightness(1.15);transform:translateY(-2px)}
  .cmap-tm-order{position:absolute;top:10px;right:10px;font-family:var(--mono);font-size:9px;font-weight:600;padding:1px 5px;border-radius:2px;border:1px solid}
  .cmap-tm-tokens{font-family:var(--mono);font-size:18px;font-weight:700;line-height:1;margin-bottom:4px}
  .cmap-tm-lines{font-family:var(--mono);font-size:10px;margin-bottom:6px}
  .cmap-tm-name{font-family:var(--mono);font-size:10px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .cmap-tm-scope{font-family:var(--mono);font-size:9px;padding:1px 5px;border-radius:2px;margin-top:8px;display:inline-block;font-weight:600}
  .cmap-tm-findings{position:absolute;top:10px;left:10px;display:flex;gap:3px}
  .cmap-lazy-section{padding:14px 20px 18px;border-top:1px solid var(--border)}
  .cmap-lazy-label{font-family:var(--mono);font-size:9px;color:var(--text3);letter-spacing:1.2px;text-transform:uppercase;margin-bottom:10px;display:flex;align-items:center;gap:8px}
  .cmap-lazy-label::after{content:'';flex:1;height:1px;background:var(--border)}
  .cmap-lazy-items{display:flex;flex-wrap:wrap;gap:6px}
  .cmap-lazy-item{display:flex;align-items:center;gap:7px;padding:7px 10px;border-radius:6px;border:1px solid var(--border);background:var(--bg3);cursor:pointer;transition:background .08s;font-family:var(--mono);font-size:11px;color:var(--text2);max-width:320px}
  .cmap-lazy-item:hover{background:var(--bg4)}
  .cmap-lazy-badge{font-size:9px;padding:1px 5px;border-radius:2px;background:var(--bg4);color:var(--text3);border:1px solid var(--border2);flex-shrink:0}
  .cmap-lazy-path{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1;min-width:0}
  .cmap-lazy-globs{font-size:9px;color:var(--text3);white-space:nowrap}
  .cmap-strip{display:flex;height:6px;border-radius:2px;overflow:hidden;gap:1px;background:var(--bg4);margin:0 20px 0}
  .cmap-strip-total{font-family:var(--mono);font-size:10px;color:var(--text3);padding:6px 20px 14px;border-bottom:1px solid var(--border)}
  /* CONTEXT MAP (sidebar mini) */
  .ctx-map{padding:8px 12px 12px}
  .ctx-chart{display:flex;align-items:flex-end;gap:3px;height:56px}
  .ctx-col{flex:1;min-width:5px;display:flex;align-items:flex-end;cursor:default;border-radius:2px 2px 0 0;overflow:hidden;transition:opacity .1s}
  .ctx-col:hover{opacity:.65}
  .ctx-col-inner{width:100%;border-radius:2px 2px 0 0}
  .ctx-baseline{height:1px;background:var(--border2);margin:0 0 6px}
  .ctx-lazy-dots{display:flex;flex-wrap:wrap;gap:3px}
  .ctx-dot{width:7px;height:7px;border-radius:1px;background:var(--bg4);border:1px solid var(--border2);cursor:default;flex-shrink:0;transition:border-color .1s}
  .ctx-dot:hover{border-color:var(--text3)}
  .ctx-dot-rule{border-style:dashed}
  .ctx-map-footer{display:flex;justify-content:space-between;margin-top:8px;font-family:var(--mono);font-size:9px;color:var(--text3)}
  /* OVERVIEW */
  .ov-status{display:flex;align-items:center;gap:14px;padding:18px 22px;border-radius:var(--radius2);border:1px solid;box-shadow:var(--shadow)}
  .ov-status-ok{background:rgba(34,197,94,.05);border-color:rgba(34,197,94,.18)}
  .ov-status-warn{background:rgba(234,179,8,.05);border-color:rgba(234,179,8,.18)}
  .ov-status-err{background:rgba(239,68,68,.05);border-color:rgba(239,68,68,.18)}
  .ov-status-dot{width:10px;height:10px;border-radius:50%;flex-shrink:0}
  .ov-status-ok .ov-status-dot{background:var(--green);box-shadow:0 0 8px rgba(34,197,94,.5)}
  .ov-status-warn .ov-status-dot{background:var(--yellow);box-shadow:0 0 8px rgba(234,179,8,.5)}
  .ov-status-err .ov-status-dot{background:var(--red);box-shadow:0 0 8px rgba(239,68,68,.5)}
  .ov-status-headline{font-size:16px;font-weight:600;color:var(--text);line-height:1.3}
  .ov-status-sub{font-size:12px;color:var(--text3);font-family:var(--mono);margin-top:3px}
  .ov-metrics{display:grid;grid-template-columns:repeat(3,1fr);gap:1px;background:var(--border)}
  .ov-metric{background:var(--bg2);padding:28px 24px 22px;display:flex;flex-direction:column;gap:8px;transition:background .1s}
  .ov-metric:hover{background:var(--bg3)}
  .ov-metric-value{font-family:var(--mono);font-size:44px;font-weight:600;line-height:1;letter-spacing:-2px}
  .ov-metric-label{font-size:10px;color:var(--text3);letter-spacing:.8px;text-transform:uppercase;font-family:var(--mono)}
  .ov-metric-sub{font-size:11px;color:var(--text3);font-family:var(--mono)}
  @media(max-width:768px){.shell{grid-template-columns:1fr}.sidebar{display:none}.ov-metrics{grid-template-columns:1fr 1fr}}
  /* ── LIGHT THEME ── */
  html[data-theme="light"]{--bg:#f5f5f3;--bg2:#ffffff;--bg3:#efeeec;--bg4:#e7e7e4;--bg5:#dededd;--border:#e2e1de;--border2:#ccccc8;--text:#18181a;--text2:#505060;--text3:#888898;--accent:#d97706;--accent2:#fffbeb;--accent3:#fde68a;--green:#16a34a;--green2:#f0fdf4;--green3:#bbf7d0;--yellow:#ca8a04;--yellow2:#fefce8;--yellow3:#fef08a;--red:#dc2626;--red2:#fef2f2;--red3:#fecaca;--purple:#7c3aed;--purple2:#f5f3ff;--dim:#b0b0be;--shadow:0 1px 3px rgba(0,0,0,.07),0 4px 12px rgba(0,0,0,.05);--shadow2:0 1px 2px rgba(0,0,0,.04)}
  @media(prefers-color-scheme:light){html[data-theme="system"]{--bg:#f5f5f3;--bg2:#ffffff;--bg3:#efeeec;--bg4:#e7e7e4;--bg5:#dededd;--border:#e2e1de;--border2:#ccccc8;--text:#18181a;--text2:#505060;--text3:#888898;--accent:#d97706;--accent2:#fffbeb;--accent3:#fde68a;--green:#16a34a;--green2:#f0fdf4;--green3:#bbf7d0;--yellow:#ca8a04;--yellow2:#fefce8;--yellow3:#fef08a;--red:#dc2626;--red2:#fef2f2;--red3:#fecaca;--purple:#7c3aed;--purple2:#f5f3ff;--dim:#b0b0be;--shadow:0 1px 3px rgba(0,0,0,.07),0 4px 12px rgba(0,0,0,.05);--shadow2:0 1px 2px rgba(0,0,0,.04)}}
  /* ── THEME TOGGLE ── */
  .theme-toggle{display:flex;background:var(--bg3);border:1px solid var(--border);border-radius:var(--radius);padding:2px;gap:1px;flex-shrink:0}
  .tt-btn{background:none;border:none;cursor:pointer;font-size:12px;color:var(--text3);padding:3px 8px;border-radius:3px;transition:background .1s,color .1s;line-height:1;font-family:var(--sans)}
  .tt-btn:hover{color:var(--text2)}
  .tt-btn.tt-active{background:var(--bg5);color:var(--text)}
</style>
<script>(function(){var t=localStorage.getItem('cmTheme')||'system';document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<div class="shell">

<header class="topbar">
  <div class="topbar-logo">claudemap <span>v{{.Version}}</span></div>
  <div class="topbar-path">{{.Workdir}}</div>
  <div class="topbar-meta">
    {{if gt .ErrCount 0}}<span class="badge badge-err">{{.ErrCount}} error{{if neq .ErrCount 1}}s{{end}}</span>{{end}}
    {{if gt .WarnCount 0}}<span class="badge badge-warn">{{.WarnCount}} warning{{if neq .WarnCount 1}}s{{end}}</span>{{end}}
    {{if gt .InfoCount 0}}<span class="badge badge-info">{{.InfoCount}} info</span>{{end}}
    {{if and (eq .ErrCount 0) (eq .WarnCount 0) (eq .InfoCount 0)}}<span class="badge badge-ok">✓ no issues</span>{{end}}
  </div>
  <div class="theme-toggle">
    <button class="tt-btn" data-theme-val="light" onclick="setTheme('light')" title="Light mode">☀</button>
    <button class="tt-btn" data-theme-val="system" onclick="setTheme('system')" title="Use system preference">◑</button>
    <button class="tt-btn" data-theme-val="dark" onclick="setTheme('dark')" title="Dark mode">☾</button>
  </div>
</header>

<nav class="sidebar">
  <div class="sidebar-section">
    <div class="sidebar-label">Views</div>
    <div class="sidebar-item active" onclick="showView('overview')">
      <span class="si-icon">◈</span><span class="si-label">Overview</span>
    </div>
    <div class="sidebar-item" onclick="showView('map')">
      <span class="si-icon">⊡</span><span class="si-label">Context Map</span>
    </div>
    <div class="sidebar-item" onclick="showView('tree')">
      <span class="si-icon">⊞</span><span class="si-label">Context Tree</span>
    </div>
    <div class="sidebar-item" onclick="showView('compose')">
      <span class="si-icon">≡</span><span class="si-label">Composed Context</span>
    </div>
    <div class="sidebar-item" onclick="showView('findings')">
      <span class="si-icon">◎</span><span class="si-label">Findings</span>
      {{if gt .ErrCount 0}}<span class="si-badge si-badge-e">{{.ErrCount}}</span>{{end}}
      {{if gt .WarnCount 0}}<span class="si-badge si-badge-w">{{.WarnCount}}</span>{{end}}
      {{if gt .InfoCount 0}}<span class="si-badge si-badge-i">{{.InfoCount}}</span>{{end}}
    </div>
  </div>
  <div class="sidebar-section">
    <div class="sidebar-label">Files — Eager</div>
    {{range $i, $f := .Assembly.EagerFiles}}
    <div class="sidebar-item" onclick="showCompose({{$i}})">
      <span class="si-icon" style="color:var(--accent)">●</span>
      <span class="si-label">{{friendlyPath $f.Path}}</span>
      <span class="si-badge si-badge-ok">~{{$f.Tokens}}</span>
    </div>
    {{end}}
  </div>
  {{if .Assembly.LazyFiles}}
  <div class="sidebar-section">
    <div class="sidebar-label">Files — Lazy</div>
    {{range $i, $f := .Assembly.LazyFiles}}
    <div class="sidebar-item" onclick="showComposeLazy({{$i}})">
      <span class="si-icon" style="color:var(--dim)">○</span>
      <span class="si-label">{{friendlyPath $f.Path}}</span>
      <span class="si-badge si-badge-i">{{if isLazy $f}}{{if eq $f.LoadTiming 1}}dir{{else}}rule{{end}}{{end}}</span>
    </div>
    {{end}}
  </div>
  {{end}}
</nav>

<main class="main">

<!-- OVERVIEW -->
<div id="view-overview">

  <!-- Health status -->
  <div class="ov-status {{if gt .ErrCount 0}}ov-status-err{{else if gt .WarnCount 0}}ov-status-warn{{else}}ov-status-ok{{end}}">
    <div class="ov-status-dot"></div>
    <div>
      <div class="ov-status-headline">{{if gt .ErrCount 0}}{{.ErrCount}} error{{if neq .ErrCount 1}}s{{end}} require attention{{else if gt .WarnCount 0}}{{.WarnCount}} warning{{if neq .WarnCount 1}}s{{end}} found{{else}}Context is healthy{{end}}</div>
      <div class="ov-status-sub">{{len .Assembly.EagerFiles}} eager · {{len .Assembly.LazyFiles}} lazy{{if gt .InfoCount 0}} · {{.InfoCount}} informational{{end}}</div>
    </div>
  </div>

  <!-- Key metrics -->
  <div class="card">
    <div class="ov-metrics">
      <div class="ov-metric">
        <div class="ov-metric-value" style="color:var(--accent)">~{{.EagerTokenTotal}}</div>
        <div class="ov-metric-label">Eager tokens</div>
        <div class="ov-metric-sub">loaded at every session start</div>
      </div>
      <div class="ov-metric">
        <div class="ov-metric-value">{{.EagerLineTotal}}</div>
        <div class="ov-metric-label">Total lines</div>
        <div class="ov-metric-sub">across {{len .Assembly.EagerFiles}} file{{if neq (len .Assembly.EagerFiles) 1}}s{{end}}</div>
      </div>
      <div class="ov-metric">
        <div class="ov-metric-value">{{add (add .ErrCount .WarnCount) .InfoCount}}</div>
        <div class="ov-metric-label">Findings</div>
        <div class="ov-metric-sub">{{.ErrCount}} err · {{.WarnCount}} warn · {{.InfoCount}} info</div>
      </div>
    </div>
  </div>

  <!-- Token distribution -->
  {{if .BarSegments}}
  <div class="card">
    <div class="card-head">
      <div class="card-title">Token distribution</div>
      <div class="card-sub">eager context only</div>
    </div>
    <div class="token-bar-wrap">
      <div class="token-bar">
        {{range .BarSegments}}<div class="tb-seg" style="flex-basis:{{printf "%.1f" .Percent}}%;background:{{.Color}};"></div>{{end}}
      </div>
      <div class="token-legend">
        {{range .BarSegments}}
        <div class="tl-item">
          <div class="tl-dot" style="background:{{.Color}};"></div>
          {{.Label}}<span class="tl-tokens">~{{.Tokens}}</span>
        </div>
        {{end}}
      </div>
    </div>
  </div>
  {{end}}

  <!-- Findings summary (compact — no detail blocks) -->
  {{if .Findings}}
  <div class="card">
    <div class="card-head">
      <div class="card-title">Findings</div>
      <div class="card-sub" style="cursor:pointer;color:var(--accent);opacity:.8" onclick="showView('findings')">view all {{len .Findings}} →</div>
    </div>
    <div class="findings-list">
      {{range .Findings}}
      <div class="finding-item" style="padding:10px 16px;">
        <div class="finding-sev"><span class="sev-icon {{sevClass .Severity}}">{{sevLabel .Severity}}</span></div>
        <div class="finding-body">
          <div class="finding-msg" style="font-size:12px;margin-bottom:2px;">{{.Message}}</div>
          <div class="finding-file" style="font-size:10px;">{{.File.RelPath}}{{if .Line}} : line {{.Line}}{{end}}</div>
        </div>
      </div>
      {{end}}
    </div>
  </div>
  {{end}}

</div>

<!-- MAP -->
<div id="view-map" style="display:none;">
  <div class="card">
    <div class="card-head">
      <div class="card-title">Context map</div>
      <div class="card-sub">area = token weight · click any block to open in composed context</div>
    </div>

    <!-- Token strip -->
    <div class="cmap-strip" style="margin-top:16px;">
      {{range .BarSegments}}<div style="flex-basis:{{printf "%.1f" .Percent}}%;background:{{.Color}};" title="{{.Label}} · ~{{.Tokens}} tokens"></div>{{end}}
    </div>
    <div class="cmap-strip-total">~{{.EagerTokenTotal}} total eager tokens · {{len .Assembly.EagerFiles}} file{{if neq (len .Assembly.EagerFiles) 1}}s{{end}} · in load order left→right</div>

    <!-- Treemap: eager files as proportional blocks -->
    <div class="cmap-treemap">
      {{range $i, $f := .Assembly.EagerFiles}}
      {{$seg := index $.BarSegments $i}}
      {{$findings := fileFindings $f.Path $.FindingsByFile}}
      <div class="cmap-tm-cell"
           style="flex-grow:{{printf "%.2f" (maxf $seg.Percent 3.0)}};border-color:{{$seg.Color}}28;background:linear-gradient(160deg,{{$seg.Color}}14 0%,{{$seg.Color}}08 100%);"
           onclick="showCompose({{$i}})"
           title="{{friendlyPath $f.Path}} · ~{{$f.Tokens}} tokens · {{$f.LineCount}} lines">
        {{if $findings}}
        <div class="cmap-tm-findings">
          {{range $findings}}<div class="tf-dot tf-dot-{{if eq .Severity "error"}}e{{else if eq .Severity "warning"}}w{{else}}i{{end}}" title="{{.Code}}: {{.Message}}"></div>{{end}}
        </div>
        {{end}}
        <div class="cmap-tm-order" style="color:{{$seg.Color}}99;border-color:{{$seg.Color}}30;">#{{$f.LoadOrder}}</div>
        <div class="cmap-tm-tokens" style="color:{{$seg.Color}};">~{{$f.Tokens}}</div>
        <div class="cmap-tm-lines" style="color:{{$seg.Color}}66;">{{$f.LineCount}}L</div>
        <div class="cmap-tm-name" style="color:var(--text3);">{{friendlyPath $f.Path}}</div>
        <span class="cmap-tm-scope {{scopeClass $f.Scope}}" style="">{{scopeLabel $f.Scope}}</span>
      </div>
      {{end}}
    </div>

    <!-- Lazy files -->
    {{if .Assembly.LazyFiles}}
    <div class="cmap-lazy-section">
      <div class="cmap-lazy-label">Lazy — loads when matching files are opened</div>
      <div class="cmap-lazy-items">
        {{range $i, $f := .Assembly.LazyFiles}}
        {{$dead := isDeadRule $f $.FindingsByFile}}
        {{$findings := fileFindings $f.Path $.FindingsByFile}}
        <div class="cmap-lazy-item" onclick="showComposeLazy({{$i}})" title="{{$f.Path}}">
          <span class="cmap-lazy-badge">{{orderLabel $f}}</span>
          <span class="cmap-lazy-path" {{if $dead}}style="color:var(--red);"{{end}}>{{friendlyPath $f.Path}}</span>
          {{if $f.PathGlobs}}<span class="cmap-lazy-globs">→ {{joinGlobs $f.PathGlobs}}</span>{{end}}
          {{range $findings}}<div class="tf-dot tf-dot-{{if eq .Severity "error"}}e{{else if eq .Severity "warning"}}w{{else}}i{{end}}" title="{{.Code}}: {{.Message}}"></div>{{end}}
        </div>
        {{end}}
      </div>
    </div>
    {{end}}

  </div>
</div>

<!-- TREE -->
<div id="view-tree" style="display:none;">
  <div class="card">
    <div class="card-head">
      <div class="card-title">CONTEXT TREE</div>
      <div class="card-sub">load order → {{.Workdir}}</div>
    </div>
    <div style="display:flex;align-items:center;gap:8px;padding:6px 14px 6px 20px;border-bottom:1px solid var(--border);background:var(--bg3);">
      <div style="font-family:var(--mono);font-size:9px;color:var(--text3);letter-spacing:1px;text-transform:uppercase;flex:1;padding-left:38px;">File</div>
      <div style="font-family:var(--mono);font-size:9px;color:var(--text3);letter-spacing:1px;text-transform:uppercase;width:70px;text-align:right;">Scope</div>
      <div style="font-family:var(--mono);font-size:9px;color:var(--text3);letter-spacing:1px;text-transform:uppercase;width:80px;text-align:right;">Timing</div>
      <div style="font-family:var(--mono);font-size:9px;color:var(--text3);letter-spacing:1px;text-transform:uppercase;width:48px;text-align:right;">Lines</div>
      <div style="font-family:var(--mono);font-size:9px;color:var(--text3);letter-spacing:1px;text-transform:uppercase;width:60px;text-align:right;">Tokens</div>
      <div style="width:20px;"></div>
    </div>
    <div class="tree">
      <div class="tree-section-divider">
        <span class="tsd-label">Eager — loads at session start</span>
        <span class="tsd-line"></span>
      </div>
      {{range .Assembly.EagerFiles}}
      {{$fp := friendlyPath .Path}}
      {{$findings := fileFindings .Path $.FindingsByFile}}
      <div class="tree-node">
        <div class="tree-row-inner">
          <div class="tree-order">{{orderLabel .}}</div>
          <div class="tree-name" title="{{.Path}}">{{$fp}}</div>
          <div class="tree-scope {{scopeClass .Scope}}" style="margin-left:auto;">{{scopeLabel .Scope}}</div>
          <div class="tree-timing" style="width:80px;text-align:right;">eager</div>
          <div class="tree-lines" {{if lineWarning .}}style="color:var(--yellow)"{{end}}>{{.LineCount}}L{{if lineWarning .}} ⚠{{end}}</div>
          <div class="tree-tokens">~{{.Tokens}}</div>
          <div class="tree-findings">
            {{range $findings}}
            <div class="tf-dot tf-dot-{{if eq .Severity "error"}}e{{else if eq .Severity "warning"}}w{{else}}i{{end}}" title="{{.Code}}: {{.Message}}"></div>
            {{end}}
          </div>
        </div>
        {{range .Imports}}
        <div class="tree-import-row">
          <div class="import-arrow">↳</div>
          {{if .IsCircular}}
          <div class="import-name" style="color:var(--yellow);">@{{.Raw}}</div>
          <div class="import-depth" style="color:var(--yellow);border-color:var(--yellow);">circular</div>
          <div class="import-arrow import-error">⚠</div>
          {{else if .ExceedsDepth}}
          <div class="import-name" style="color:var(--red);">@{{.Raw}}</div>
          <div class="import-depth" style="color:var(--red);border-color:var(--red);">depth:{{.Depth}}</div>
          <div class="import-arrow import-error">depth exceeded</div>
          {{else if not .Exists}}
          <div class="import-name" style="color:var(--red);">@{{.Raw}}</div>
          <div class="import-depth" style="color:var(--red);border-color:var(--red);">depth:{{.Depth}}</div>
          <div class="import-arrow import-error">✗ not found</div>
          {{else}}
          <div class="import-name">@{{.Raw}}</div>
          <div class="import-depth">depth:{{.Depth}}</div>
          <div class="import-arrow import-ok">✓</div>
          {{end}}
        </div>
        {{end}}
      </div>
      {{end}}
      <div style="display:flex;align-items:center;gap:8px;padding:10px 20px;border-top:1px solid var(--border);background:var(--bg3);margin-top:4px;">
        <div style="flex:1;font-family:var(--mono);font-size:9px;color:var(--text3);letter-spacing:1px;text-transform:uppercase;">Total Eager</div>
        <div style="font-family:var(--mono);font-size:11px;color:var(--text);width:48px;text-align:right;font-weight:600;">{{.EagerLineTotal}}L</div>
        <div style="font-family:var(--mono);font-size:11px;color:var(--accent);width:60px;text-align:right;font-weight:600;">~{{.EagerTokenTotal}}</div>
        <div style="width:20px;"></div>
      </div>
      {{if .Assembly.LazyFiles}}
      <div class="tree-section-divider" style="margin-top:8px;">
        <span class="tsd-label">Lazy — loads when matching files opened</span>
        <span class="tsd-line"></span>
      </div>
      {{range .Assembly.LazyFiles}}
      {{$findings := fileFindings .Path $.FindingsByFile}}
      {{$dead := isDeadRule . $.FindingsByFile}}
      <div class="tree-node">
        <div class="tree-row-inner" {{if $dead}}style="opacity:.7;"{{end}}>
          <div class="tree-order" {{if $dead}}style="color:var(--red);border-color:var(--red);"{{end}}>{{orderLabel .}}</div>
          <div class="tree-name" {{if $dead}}style="color:var(--red);"{{end}} title="{{.Path}}">{{friendlyPath .Path}}</div>
          <div class="tree-scope {{if $dead}}scope-rule" style="background:var(--red2);color:var(--red);border:1px solid var(--red);margin-left:auto;{{else}}{{scopeClass .Scope}}" style="margin-left:auto;{{end}}">{{scopeLabel .Scope}}</div>
          <div class="tree-timing" style="width:80px;text-align:right;{{if $dead}}color:var(--red);{{end}}">{{timingLabel .}}</div>
          <div class="tree-lines" style="color:var(--text3);">{{.LineCount}}L</div>
          <div class="tree-tokens" style="color:var(--text3);">~{{.Tokens}}</div>
          <div class="tree-findings">
            {{range $findings}}
            <div class="tf-dot tf-dot-{{if eq .Severity "error"}}e{{else if eq .Severity "warning"}}w{{else}}i{{end}}" title="{{.Code}}: {{.Message}}"></div>
            {{end}}
          </div>
        </div>
      </div>
      {{end}}
      {{end}}
    </div>
  </div>
</div>

<!-- COMPOSE -->
<div id="view-compose" style="display:none;">
  <div class="card">
    <div class="card-head">
      <div class="card-title">Composed context</div>
      <div class="card-sub">~{{.EagerTokenTotal}} tokens eager · {{len .Assembly.ComposedBlocks}} block{{if neq (len .Assembly.ComposedBlocks) 1}}s{{end}}</div>
      <div class="compose-view-toggle" style="margin-left:auto;">
        <button class="cvt-btn cvt-active" data-view="blocks" onclick="setComposeView('blocks')">Blocks</button>
        <button class="cvt-btn" data-view="eager" onclick="setComposeView('eager')">Eager</button>
        <button class="cvt-btn" data-view="all" onclick="setComposeView('all')">All</button>
      </div>
      <button class="compose-copy-btn" id="compose-copy-btn" onclick="copyComposedContext()" title="Copy full context to clipboard">⎘ Copy all</button>
    </div>

    <!-- BLOCKS VIEW -->
    <div id="compose-view-blocks">
      {{if .Assembly.ComposedBlocks}}
      <div class="compose-section-header">
        <span>Eager — loaded at session start</span>
      </div>
      {{end}}
      {{range $i, $b := .Assembly.ComposedBlocks}}
      <div class="compose-block" id="compose-block-{{$i}}">
        <div class="compose-header" onclick="toggleCompose(this)">
          <div class="ch-order">#{{$b.Source.LoadOrder}}</div>
          <div class="ch-file">{{friendlyPath $b.Source.Path}} <span style="color:var(--text3);font-size:9px;margin-left:6px;">{{scopeLabel $b.Source.Scope}}</span></div>
          <div class="ch-tokens">~{{$b.Tokens}} tokens</div>
          <div class="ch-toggle">▶</div>
        </div>
        <div class="compose-content">
          <pre>{{$b.Content}}</pre>
        </div>
      </div>
      {{end}}
      {{if .Assembly.LazyFiles}}
      <div class="compose-section-header" style="margin-top:4px;">
        <span>Lazy — loads when matching files are opened</span>
      </div>
      {{range $i, $f := .Assembly.LazyFiles}}
      <div class="compose-block" id="compose-lazy-{{$i}}">
        <div class="compose-header" onclick="toggleCompose(this)" style="opacity:.85;">
          <div class="ch-order" style="color:var(--text3);">{{orderLabel $f}}</div>
          <div class="ch-file" style="color:var(--text2);">{{friendlyPath $f.Path}} <span style="color:var(--text3);font-size:9px;margin-left:6px;">{{scopeLabel $f.Scope}}{{if $f.PathGlobs}} · {{joinGlobs $f.PathGlobs}}{{end}}</span></div>
          <div class="ch-tokens">~{{$f.Tokens}} tokens · {{$f.LineCount}}L</div>
          <div class="ch-toggle">▶</div>
        </div>
        <div class="compose-content">
          {{if $f.CleanContent}}<pre>{{$f.CleanContent}}</pre>{{else}}<div style="padding:16px;font-family:var(--mono);font-size:11px;color:var(--text3);">— no content —</div>{{end}}
        </div>
      </div>
      {{end}}
      {{end}}
    </div>

    <!-- EAGER COMBINED VIEW -->
    <div id="compose-view-eager" style="display:none;">
      <div class="compose-combined-wrap">
        {{range $i, $b := .Assembly.ComposedBlocks}}
        <div class="compose-combined-sep">
          <span class="ccs-order">#{{$b.Source.LoadOrder}}</span>
          <span class="ccs-file">{{friendlyPath $b.Source.Path}}</span>
          <span class="ccs-scope">{{scopeLabel $b.Source.Scope}}</span>
          <span class="ccs-tokens">~{{$b.Tokens}} tokens</span>
        </div>
        <pre class="compose-combined-pre">{{$b.Content}}</pre>
        {{end}}
        {{if .Assembly.LazyFiles}}
        <div class="compose-combined-lazy-note">
          + {{len .Assembly.LazyFiles}} lazy file{{if neq (len .Assembly.LazyFiles) 1}}s{{end}} not included — switch to All to see them
        </div>
        {{end}}
      </div>
    </div>

    <!-- ALL COMBINED VIEW (eager + lazy) -->
    <div id="compose-view-all" style="display:none;">
      <div class="compose-combined-wrap">
        {{if .Assembly.ComposedBlocks}}
        <div class="compose-combined-sep" style="background:var(--accent2);border-color:var(--accent3);">
          <span class="ccs-order" style="color:var(--accent);border-color:var(--accent3);">eager</span>
          <span class="ccs-file" style="color:var(--accent);">Loaded at session start</span>
          <span class="ccs-tokens" style="color:var(--accent);">~{{.EagerTokenTotal}} tokens · {{len .Assembly.ComposedBlocks}} file{{if neq (len .Assembly.ComposedBlocks) 1}}s{{end}}</span>
        </div>
        {{range $i, $b := .Assembly.ComposedBlocks}}
        <div class="compose-combined-sep">
          <span class="ccs-order">#{{$b.Source.LoadOrder}}</span>
          <span class="ccs-file">{{friendlyPath $b.Source.Path}}</span>
          <span class="ccs-scope">{{scopeLabel $b.Source.Scope}}</span>
          <span class="ccs-tokens">~{{$b.Tokens}} tokens</span>
        </div>
        <pre class="compose-combined-pre">{{$b.Content}}</pre>
        {{end}}
        {{end}}
        {{if .Assembly.LazyFiles}}
        <div class="compose-combined-sep" style="background:var(--bg4);margin-top:4px;">
          <span class="ccs-order" style="color:var(--dim);border-color:var(--border2);">lazy</span>
          <span class="ccs-file" style="color:var(--text3);">Loaded when matching files are opened</span>
          <span class="ccs-tokens">{{len .Assembly.LazyFiles}} file{{if neq (len .Assembly.LazyFiles) 1}}s{{end}}</span>
        </div>
        {{range $i, $f := .Assembly.LazyFiles}}
        <div class="compose-combined-sep" style="opacity:.75;">
          <span class="ccs-order" style="color:var(--text3);border-color:var(--border);">{{orderLabel $f}}</span>
          <span class="ccs-file" style="color:var(--text2);">{{friendlyPath $f.Path}}</span>
          <span class="ccs-scope">{{scopeLabel $f.Scope}}{{if $f.PathGlobs}} · {{joinGlobs $f.PathGlobs}}{{end}}</span>
          <span class="ccs-tokens">~{{$f.Tokens}} tokens · {{$f.LineCount}}L</span>
        </div>
        {{if $f.CleanContent}}<pre class="compose-combined-pre" style="opacity:.8;">{{$f.CleanContent}}</pre>{{end}}
        {{end}}
        {{end}}
      </div>
    </div>

  </div>
</div>

<!-- FINDINGS -->
<div id="view-findings" style="display:none;">
  <div class="card">
    <div class="card-head">
      <div class="card-title">ALL FINDINGS</div>
      <div class="card-sub">{{.ErrCount}} error{{if neq .ErrCount 1}}s{{end}} · {{.WarnCount}} warning{{if neq .WarnCount 1}}s{{end}} · {{.InfoCount}} info</div>
    </div>
    <div class="tabs">
      <div class="tab active" onclick="filterFindings('all',this)">All ({{len .Findings}})</div>
      <div class="tab" onclick="filterFindings('error',this)">Errors ({{.ErrCount}})</div>
      <div class="tab" onclick="filterFindings('warning',this)">Warnings ({{.WarnCount}})</div>
      <div class="tab" onclick="filterFindings('info',this)">Info ({{.InfoCount}})</div>
    </div>
    <div class="findings-list" id="findings-list">
      {{range .Findings}}
      <div class="finding-item" data-sev="{{.Severity}}">
        <div class="finding-sev"><span class="sev-icon {{sevClass .Severity}}">{{sevLabel .Severity}}</span></div>
        <div class="finding-body">
          <div class="finding-code">{{.Code}}</div>
          <div class="finding-msg">{{.Message}}</div>
          <div class="finding-file">{{.File.RelPath}}{{if .Line}} : line {{.Line}}{{end}}</div>
          {{if .Detail}}<div class="finding-detail">{{.Detail}}</div>{{end}}
        </div>
      </div>
      {{end}}
      {{if not .Findings}}
      <div style="padding:24px 16px;font-family:var(--mono);font-size:11px;color:var(--text3);text-align:center;">✓ no findings</div>
      {{end}}
    </div>
  </div>
</div>

</main>
</div>

<script>
function setTheme(t){
  localStorage.setItem('cmTheme',t);
  document.documentElement.setAttribute('data-theme',t);
  document.querySelectorAll('.tt-btn').forEach(b=>b.classList.toggle('tt-active',b.dataset.themeVal===t));
}
(function(){
  var t=localStorage.getItem('cmTheme')||'system';
  document.querySelectorAll('.tt-btn').forEach(b=>b.classList.toggle('tt-active',b.dataset.themeVal===t));
})();
const views=['overview','map','tree','compose','findings'];
function showView(name){
  views.forEach(v=>{
    document.getElementById('view-'+v).style.display=(v===name)?'':'none';
  });
  document.querySelectorAll('.sidebar-item').forEach(el=>{
    el.classList.toggle('active',el.getAttribute('onclick')===("showView('"+name+"')"));
  });
}
function escHtml(s){return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')}
function initLineNumbers(){
  document.querySelectorAll('.compose-content pre,.compose-combined-pre').forEach(pre=>{
    if(pre.dataset.lnDone) return;
    pre.dataset.lnDone='1';
    const raw=pre.textContent;
    pre.dataset.rawContent=raw;
    const lines=raw.split('\n');
    if(lines.length>0&&lines[lines.length-1]==='') lines.pop();
    const w=String(lines.length).length;
    pre.style.setProperty('--ln-w',(w+1)+'ch');
    pre.innerHTML=lines.map((l,i)=>
      '<span class="ln"><span class="ln-num">'+(i+1)+'</span><span class="ln-text">'+escHtml(l)+'</span></span>'
    ).join('');
  });
}
document.addEventListener('DOMContentLoaded',initLineNumbers);
function setComposeView(v){
  ['blocks','eager','all'].forEach(n=>{
    document.getElementById('compose-view-'+n).style.display=v===n?'':'none';
  });
  document.querySelectorAll('.cvt-btn').forEach(b=>b.classList.toggle('cvt-active',b.dataset.view===v));
}
function copyComposedContext(){
  const activeView=['eager','all'].find(n=>document.getElementById('compose-view-'+n).style.display!=='none')||'eager';
  const pres=document.querySelectorAll('#compose-view-'+activeView+' .compose-combined-pre');
  const seps=document.querySelectorAll('#compose-view-'+activeView+' .compose-combined-sep');
  let out='';
  // interleave sep labels with pre content
  document.querySelectorAll('#compose-view-'+activeView+' > .compose-combined-wrap > *').forEach(el=>{
    if(el.classList.contains('compose-combined-sep')){
      const file=el.querySelector('.ccs-file');
      const order=el.querySelector('.ccs-order');
      if(file&&order&&!el.style.background.includes('accent')){
        out+='<!-- '+order.textContent.trim()+' '+file.textContent.trim()+' -->\n';
      }
    } else if(el.tagName==='PRE'){
      out+=(el.dataset.rawContent||el.textContent)+'\n\n';
    }
  });
  navigator.clipboard.writeText(out.trimEnd()).then(()=>{
    const btn=document.getElementById('compose-copy-btn');
    btn.classList.add('copied');
    btn.textContent='✓ Copied';
    setTimeout(()=>{btn.classList.remove('copied');btn.innerHTML='⎘ Copy all';},2000);
  });
}
function showCompose(idx){
  showView('compose');
  setComposeView('blocks');
  // close all blocks
  document.querySelectorAll('.compose-block').forEach(b=>{
    b.querySelector('.compose-content').classList.remove('open');
    b.querySelector('.ch-toggle').textContent='▶';
  });
  // open the target
  const block=document.getElementById('compose-block-'+idx);
  if(!block) return;
  block.querySelector('.compose-content').classList.add('open');
  block.querySelector('.ch-toggle').textContent='▼';
  block.scrollIntoView({behavior:'smooth',block:'start'});
  document.querySelectorAll('.sidebar-item').forEach(el=>{
    el.classList.toggle('active',el.getAttribute('onclick')===('showCompose('+idx+')'));
  });
}
function toggleCompose(header){
  const content=header.nextElementSibling;
  const toggle=header.querySelector('.ch-toggle');
  const isOpen=content.classList.toggle('open');
  toggle.textContent=isOpen?'▼':'▶';
  if(isOpen) initLineNumbers();
}
function showComposeLazy(idx){
  showView('compose');
  document.querySelectorAll('.compose-block').forEach(b=>{
    b.querySelector('.compose-content').classList.remove('open');
    b.querySelector('.ch-toggle').textContent='▶';
  });
  const block=document.getElementById('compose-lazy-'+idx);
  if(!block) return;
  block.querySelector('.compose-content').classList.add('open');
  block.querySelector('.ch-toggle').textContent='▼';
  block.scrollIntoView({behavior:'smooth',block:'start'});
  document.querySelectorAll('.sidebar-item').forEach(el=>{
    el.classList.toggle('active',el.getAttribute('onclick')===('showComposeLazy('+idx+')'));
  });
}
function filterFindings(sev,tabEl){
  document.querySelectorAll('.tab').forEach(t=>t.classList.remove('active'));
  tabEl.classList.add('active');
  document.querySelectorAll('#findings-list .finding-item').forEach(item=>{
    item.style.display=(sev==='all'||item.dataset.sev===sev)?'':'none';
  });
}
</script>
</body>
</html>`
