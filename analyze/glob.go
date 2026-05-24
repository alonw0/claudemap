package analyze

import (
	"path/filepath"
	"strings"
)

// GlobMatch reports whether path matches the doublestar glob pattern.
// Supports ** (any path depth), * (within a segment), and ? (single char).
func GlobMatch(pattern, path string) bool {
	// Normalize separators
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)
	return matchGlob(pattern, path)
}

func matchGlob(pattern, str string) bool {
	for len(pattern) > 0 {
		if strings.HasPrefix(pattern, "**/") {
			// ** at start: try matching zero or more path segments
			rest := pattern[3:]
			// Try matching rest against str as-is (zero segments consumed)
			if matchGlob(rest, str) {
				return true
			}
			// Consume one segment of str and try again
			idx := strings.Index(str, "/")
			if idx < 0 {
				return false
			}
			return matchGlob(pattern, str[idx+1:])
		}

		if pattern == "**" {
			// Match everything remaining
			return true
		}

		// Split into head segment and rest
		pSep := strings.Index(pattern, "/")
		sSep := strings.Index(str, "/")

		var pSeg, pRest string
		if pSep < 0 {
			pSeg = pattern
			pRest = ""
		} else {
			pSeg = pattern[:pSep]
			pRest = pattern[pSep+1:]
		}

		var sSeg, sRest string
		if sSep < 0 {
			sSeg = str
			sRest = ""
		} else {
			sSeg = str[:sSep]
			sRest = str[sSep+1:]
		}

		if pSep >= 0 && sSep < 0 {
			// pattern has more segments but str doesn't
			return false
		}

		if !matchSegment(pSeg, sSeg) {
			return false
		}

		if pSep < 0 {
			// Pattern segment matched; make sure str has no more segments
			return sSep < 0
		}

		pattern = pRest
		str = sRest
	}
	return len(str) == 0
}

func matchSegment(pattern, s string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Consume all consecutive *
			pattern = strings.TrimLeft(pattern, "*")
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if matchSegment(pattern, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		default:
			if len(s) == 0 || pattern[0] != s[0] {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		}
	}
	return len(s) == 0
}

// GlobMatchAny reports whether path matches any of the given glob patterns.
func GlobMatchAny(patterns []string, path string) bool {
	for _, p := range patterns {
		if GlobMatch(p, path) {
			return true
		}
	}
	return false
}
