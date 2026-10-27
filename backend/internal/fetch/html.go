package fetch

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	tagPattern    = regexp.MustCompile(`<[^>]*>`)
	entityPattern = regexp.MustCompile(`&(#[xX][0-9a-fA-F]+|#[0-9]+|[a-zA-Z]+);`)
	cdataPattern  = regexp.MustCompile(`(?s)<!\[CDATA\[(.*?)\]\]>`)
	entities      = map[string]string{"nbsp": " ", "amp": "&", "lt": "<", "gt": ">", "quot": `"`, "apos": "'"}
)

// StripHTML removes tags, decodes common entities and collapses whitespace.
func StripHTML(v string) string {
	v = cdataPattern.ReplaceAllString(v, "$1")
	v = tagPattern.ReplaceAllString(v, "")
	v = entityPattern.ReplaceAllStringFunc(v, func(m string) string {
		e := strings.ToLower(m[1 : len(m)-1])
		var cp int64
		var err error
		switch {
		case strings.HasPrefix(e, "#x"):
			cp, err = strconv.ParseInt(e[2:], 16, 32)
		case strings.HasPrefix(e, "#"):
			cp, err = strconv.ParseInt(e[1:], 10, 32)
		default:
			if s, ok := entities[e]; ok {
				return s
			}
			return m
		}
		if err != nil || cp <= 0 || cp > 0x10FFFF {
			return m
		}
		return string(rune(cp))
	})
	return strings.Join(strings.Fields(v), " ")
}
