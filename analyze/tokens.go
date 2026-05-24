package analyze

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

var codeBlockRe = regexp.MustCompile("(?s)```[\\s\\S]*?```")
var commentRe = regexp.MustCompile("(?s)<!--[\\s\\S]*?-->")

func StripHTMLComments(s string) string {
	i := 0
	placeholders := map[string]string{}
	result := codeBlockRe.ReplaceAllStringFunc(s, func(match string) string {
		key := fmt.Sprintf("__CODEBLOCK_%d__", i)
		placeholders[key] = match
		i++
		return key
	})
	result = commentRe.ReplaceAllString(result, "")
	for key, val := range placeholders {
		result = strings.ReplaceAll(result, key, val)
	}
	return result
}

func EstimateTokens(raw string) int {
	clean := StripHTMLComments(raw)
	codeBlocks := codeBlockRe.FindAllString(clean, -1)
	codeChars := 0
	for _, b := range codeBlocks {
		codeChars += len(b)
	}
	proseChars := len(clean) - codeChars
	tokens := float64(proseChars)/4.0 + float64(codeChars)/3.5
	return int(math.Ceil(tokens))
}
