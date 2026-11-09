package bookmarks

import (
	"regexp"
	"strings"

	"golang.org/x/net/idna"
)

var commentRe = regexp.MustCompile(`\s+#`)

// AutoTags is a port of linkding's services/auto_tagging.py. Each rule line is
// "<url-pattern> <tag> [<tag>...]" with optional "# comments". The pattern host
// matches the URL host or any parent-domain suffix match, the path and fragment
// are prefix matches, and every query parameter of the pattern must be present
// in the URL (an empty pattern value matches any value). Matching is
// case-insensitive. The result is passed through NormalizeTags.
func AutoTags(rules, rawURL string) []string {
	u := splitURL(strings.ToLower(rawURL))
	if u.hostname == "" {
		return []string{}
	}
	var tags []string
	for _, line := range strings.Split(strings.ToLower(rules), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if loc := commentRe.FindStringIndex(line); loc != nil {
			line = line[:loc[0]]
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		pat := parts[0]
		for _, pre := range []string{"https://", "http://"} {
			if strings.HasPrefix(pat, pre) {
				pat = pat[len(pre):]
				break
			}
		}
		p := splitURL("//" + pat)
		if p.hostname == "" || !domainMatches(p.hostname, u.hostname) {
			continue
		}
		if p.path != "" && !strings.HasPrefix(u.path, p.path) {
			continue
		}
		if p.query != "" && !queryMatches(p.query, u.query) {
			continue
		}
		if p.fragment != "" && !strings.HasPrefix(u.fragment, p.fragment) {
			continue
		}
		tags = append(tags, parts[1:]...)
	}
	return NormalizeTags(tags)
}

func domainMatches(expected, actual string) bool {
	return strings.HasSuffix(toASCII(actual), toASCII(expected))
}

func toASCII(host string) string {
	if a, err := idna.Lookup.ToASCII(host); err == nil {
		return a
	}
	return host
}

func queryMatches(expected, actual string) bool {
	exp, act := parseQS(expected), parseQS(actual)
	for k, vals := range exp {
		have, ok := act[k]
		if !ok {
			return false
		}
		for _, v := range vals {
			if v == "" {
				continue
			}
			found := false
			for _, h := range have {
				if h == v {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}
