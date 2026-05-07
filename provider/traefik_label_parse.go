package provider

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
	"github.com/samber/mo"
)

var traefikRuleCallPattern = regexp.MustCompile(`(?i)(Host|PathPrefix|Path|Method|Headers?)\s*\(([^)]*)\)`)

func applyTraefikRule(router *TraefikRouter, rule string) {
	for _, match := range traefikRuleCallPattern.FindAllStringSubmatch(rule, -1) {
		args := traefikRuleArgs(match[2])
		if len(args) == 0 {
			continue
		}
		switch strings.ToLower(match[1]) {
		case "host":
			router.Host = args[0]
		case "path", "pathprefix":
			router.PathPrefix = args[0]
		case "method":
			router.Method = strings.ToUpper(args[0])
		case "header", "headers":
			for idx := 0; idx+1 < len(args); idx += 2 {
				router.Headers.Set(args[idx], args[idx+1])
			}
		}
	}
}

func traefikRuleArgs(input string) []string {
	scanner := traefikRuleArgScanner{args: collectionlist.NewList[string]()}
	for _, current := range input {
		scanner.consume(current)
	}
	return scanner.args.Values()
}

type traefikRuleArgScanner struct {
	args    *collectionlist.List[string]
	builder strings.Builder
	quote   rune
}

func (s *traefikRuleArgScanner) consume(current rune) {
	if s.quote == 0 {
		s.open(current)
		return
	}
	if current == s.quote {
		s.close()
		return
	}
	if _, err := s.builder.WriteRune(current); err != nil {
		return
	}
}

func (s *traefikRuleArgScanner) open(current rune) {
	if current != '`' && current != '\'' && current != '"' {
		return
	}
	s.quote = current
	s.builder.Reset()
}

func (s *traefikRuleArgScanner) close() {
	if value := strings.TrimSpace(s.builder.String()); value != "" {
		s.args.Add(value)
	}
	s.quote = 0
}

func splitTraefikResourceLabel(rest string) (string, string, bool) {
	name, option, ok := strings.Cut(rest, ".")
	if !ok {
		return "", "", false
	}
	name = strings.TrimSpace(name)
	option = strings.TrimSpace(option)
	return name, option, name != "" && option != ""
}

func normalizeTraefikLabels(labels map[string]string) *mapping.Map[string, string] {
	normalized := mapping.NewMap[string, string]()
	for key, value := range labels {
		normalized.Set(strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value))
	}
	return normalized
}

func traefikCSVList(value string, stripNamespace bool) *collectionlist.List[string] {
	return collectionlist.FilterMapList(collectionlist.NewList(SplitCSV(value)...), func(_ int, item string) (string, bool) {
		if stripNamespace {
			item = StripTraefikProviderNamespace(item)
		}
		return item, item != ""
	})
}

func firstTraefikCSV(value string) string {
	return collectionlist.NewList(SplitCSV(value)...).
		FirstWhere(func(_ int, _ string) bool { return true }).
		OrElse("")
}

func setHeader(headers map[string]string, key, value string) {
	mo.EmptyableToOption(strings.TrimSpace(key)).ForEach(func(header string) {
		headers[header] = strings.TrimSpace(value)
	})
}

func applyTraefikSecurityHeader(middleware *config.Middleware, option, value string) {
	switch option {
	case "headers.framedeny":
		if parseTraefikBool(value) {
			middleware.ResponseHeaders["x-frame-options"] = "DENY"
		}
	case "headers.contenttypenosniff":
		if parseTraefikBool(value) {
			middleware.ResponseHeaders["x-content-type-options"] = "nosniff"
		}
	case "headers.browserxssfilter":
		if parseTraefikBool(value) {
			middleware.ResponseHeaders["x-xss-protection"] = "1; mode=block"
		}
	case "headers.stsseconds":
		if seconds := parseTraefikInt(value, 0); seconds > 0 {
			middleware.ResponseHeaders["strict-transport-security"] = fmt.Sprintf("max-age=%d", seconds)
		}
	case "headers.referrerpolicy":
		setHeader(middleware.ResponseHeaders, "referrer-policy", value)
	}
}

func parseTraefikBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return mo.TupleToOption(parsed, err == nil).OrElse(false)
}

func parseTraefikInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return mo.TupleToOption(parsed, err == nil).OrElse(fallback)
}
