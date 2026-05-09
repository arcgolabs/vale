package runtime

import (
	"strings"
	"unicode/utf8"
)

func normalizeRequestHost(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return hostPort
	}
	return strings.ToLower(requestHostWithoutPort(hostPort))
}

func requestHostWithoutPort(hostPort string) string {
	if strings.HasPrefix(hostPort, "[") {
		end := strings.IndexByte(hostPort, ']')
		if end > 0 {
			return hostPort[1:end]
		}
		return hostPort
	}
	colon := strings.LastIndexByte(hostPort, ':')
	if colon < 0 || colon == len(hostPort)-1 || strings.Contains(hostPort[:colon], ":") {
		return hostPort
	}
	return hostPort[:colon]
}

func normalizeRequestMethod(method string) string {
	for idx := range len(method) {
		if method[idx] >= 'a' && method[idx] <= 'z' {
			return strings.ToUpper(method)
		}
	}
	return method
}

func trimLastRune(value string) string {
	if value == "" {
		return ""
	}
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
}

func reverseString(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	for left, right := 0, len(runes)-1; left < right; left, right = left+1, right-1 {
		runes[left], runes[right] = runes[right], runes[left]
	}
	return string(runes)
}
