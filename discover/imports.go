package discover

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alonw0/claudemap/model"
)

var importRe = regexp.MustCompile(`@([^\s]+)`)

func resolveImports(filePath, rawContent string, depth int, seen map[string]bool) []model.ImportRef {
	var refs []model.ImportRef
	lines := strings.Split(rawContent, "\n")
	inCodeBlock := false

	for lineNum, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
		}
		if inCodeBlock {
			continue
		}

		matches := importRe.FindAllStringSubmatchIndex(line, -1)
		for _, m := range matches {
			raw := line[m[2]:m[3]]
			ref := model.ImportRef{
				Raw:   raw,
				Depth: depth,
				Line:  lineNum + 1,
			}

			absPath, err := filepath.Abs(filepath.Join(filepath.Dir(filePath), raw))
			if err != nil {
				ref.Exists = false
				refs = append(refs, ref)
				continue
			}
			ref.Resolved = absPath

			if seen[absPath] {
				ref.IsCircular = true
				refs = append(refs, ref)
				continue
			}

			if depth >= 5 {
				ref.ExceedsDepth = true
				refs = append(refs, ref)
				continue
			}

			content, err := os.ReadFile(absPath)
			if err != nil {
				ref.Exists = false
				refs = append(refs, ref)
				continue
			}
			ref.Exists = true

			childSeen := make(map[string]bool, len(seen)+1)
			for k, v := range seen {
				childSeen[k] = v
			}
			childSeen[absPath] = true

			ref.Children = resolveImports(absPath, string(content), depth+1, childSeen)
			refs = append(refs, ref)
		}
	}
	return refs
}
