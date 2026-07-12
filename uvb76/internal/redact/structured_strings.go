package redact

import (
	"encoding/json"
	"strings"
)

// redactStructuredString sanitizes secret-bearing content nested inside an
// otherwise non-sensitive JSON string field. This covers embedded JSON and
// typed HTTP header lines without treating arbitrary prose as credentials.
func redactStructuredString(value string) string {
	if embedded, ok := redactEmbeddedJSON(value); ok {
		return embedded
	}
	lines := strings.Split(value, "\n")
	changed := false
	for i, line := range lines {
		redacted, ok := redactHeaderLine(line)
		if ok {
			lines[i] = redacted
			changed = true
		}
	}
	if changed {
		return strings.Join(lines, "\n")
	}
	return value
}

func redactEmbeddedJSON(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return "", false
	}
	var embedded any
	if err := json.Unmarshal([]byte(trimmed), &embedded); err != nil {
		return "", false
	}
	sanitized, err := json.Marshal(redactValue(embedded))
	if err != nil {
		return Redacted, true
	}
	return string(sanitized), true
}

func redactHeaderLine(line string) (string, bool) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return line, false
	}
	name := strings.ToLower(strings.TrimSpace(line[:colon]))
	value := strings.TrimSpace(line[colon+1:])
	var redacted string
	switch name {
	case "authorization":
		redacted = RedactAuthorizationHeader(value)
	case "proxy-authorization":
		redacted = RedactProxyAuthorizationHeader(value)
	case "x-api-key":
		redacted = RedactAPIKeyHeader(value)
	case "x-session-token":
		redacted = RedactSessionTokenHeader(value)
	case "cookie":
		redacted = RedactRequestCookieHeader(value)
	case "set-cookie":
		redacted = RedactSetCookieHeader(value)
	default:
		return line, false
	}
	return line[:colon+1] + " " + redacted, true
}
