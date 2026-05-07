package provider

import (
	"regexp"
	"strconv"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
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
	items := collectionlist.NewList[string]()
	for _, item := range SplitCSV(value) {
		if stripNamespace {
			item = StripTraefikProviderNamespace(item)
		}
		if item != "" {
			items.Add(item)
		}
	}
	return items
}

func firstTraefikCSV(value string) string {
	for _, item := range SplitCSV(value) {
		return item
	}
	return ""
}

func setHeader(headers map[string]string, key, value string) {
	if strings.TrimSpace(key) != "" {
		headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

func parseTraefikBool(value string, fallback bool) bool {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	return mo.TupleToOption(parsed, err == nil).OrElse(fallback)
}

func parseTraefikInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	return mo.TupleToOption(parsed, err == nil).OrElse(fallback)
}
