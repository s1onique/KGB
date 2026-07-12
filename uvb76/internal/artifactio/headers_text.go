package artifactio

import (
	"strings"

	"github.com/s1onique/KGB/uvb76/internal/redact"
)

// redactHeaderLinesImpl implements per-line header redaction for textual
// documents emitted by lab tooling. Each "Header-Name: value" line is
// routed through the typed header redactor for that header name.
func redactHeaderLinesImpl(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = redactHeaderLine(line)
	}
	return strings.Join(lines, "\n")
}

// redactHeaderLine rewrites a single "Header-Name: value" line through
// the typed redactor matching the header name. Lines that do not look
// like headers are returned unchanged.
func redactHeaderLine(line string) string {
	colonIdx := strings.Index(line, ":")
	if colonIdx <= 0 {
		return line
	}
	name := strings.TrimSpace(line[:colonIdx])
	value := line[colonIdx+1:]
	if hasLeadingWhitespace(line) {
		prefix := line[:strings.Index(line, name)]
		return prefix + redactHeaderPair(name, value)
	}
	return name + ":" + redactHeaderPair(name, value)
}

// hasLeadingWhitespace reports whether the line starts with a space or tab.
func hasLeadingWhitespace(line string) bool {
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == ' ' || c == '\t' {
			return true
		}
		return false
	}
	return false
}

// redactHeaderPair dispatches a header name/value pair to the typed
// redactor matching the header name. Unknown names are returned as-is.
func redactHeaderPair(name, value string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization":
		v := strings.TrimLeft(value, " \t")
		low := strings.ToLower(v)
		switch {
		case strings.HasPrefix(low, "bearer "):
			return " " + redact.RedactAuthorizationHeader(v)
		case strings.HasPrefix(low, "basic "):
			return " " + redact.RedactAuthorizationHeader(v)
		default:
			return " " + redact.Redacted
		}
	case "proxy-authorization":
		return " " + redact.RedactProxyAuthorizationHeader(strings.TrimLeft(value, " \t"))
	case "x-api-key":
		return " " + redact.RedactAPIKeyHeader(strings.TrimLeft(value, " \t"))
	case "x-session-token":
		return " " + redact.RedactSessionTokenHeader(strings.TrimLeft(value, " \t"))
	}
	return value
}

// redactCookieLinesImpl implements per-line cookie redaction for textual
// documents. Both "Cookie: ..." and "Set-Cookie: ..." lines are routed
// through their respective typed redactor.
func redactCookieLinesImpl(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = redactCookieLine(line)
	}
	return strings.Join(lines, "\n")
}

// redactCookieLine rewrites a single header line if it carries a
// Cookie or Set-Cookie header.
func redactCookieLine(line string) string {
	colonIdx := strings.Index(line, ":")
	if colonIdx <= 0 {
		return line
	}
	name := strings.TrimSpace(line[:colonIdx])
	value := line[colonIdx+1:]
	switch strings.ToLower(name) {
	case "cookie":
		if hasLeadingWhitespace(line) {
			prefix := line[:strings.Index(line, name)]
			return prefix + name + ":" + " " + redact.RedactRequestCookieHeader(strings.TrimLeft(value, " \t"))
		}
		return name + ":" + " " + redact.RedactRequestCookieHeader(strings.TrimLeft(value, " \t"))
	case "set-cookie":
		if hasLeadingWhitespace(line) {
			prefix := line[:strings.Index(line, name)]
			return prefix + name + ":" + " " + redact.RedactSetCookieHeader(strings.TrimLeft(value, " \t"))
		}
		return name + ":" + " " + redact.RedactSetCookieHeader(strings.TrimLeft(value, " \t"))
	}
	return line
}

// redactURLInLine rewrites every URL embedded in a line via the typed
// URL redactor. The redactor fails closed for malformed URLs.
func redactURLInLine(line string) string {
	return redactURLsImpl(line)
}

// redactURLsImpl performs in-place URL redaction on the line by scanning
// for "scheme://..." occurrences and replacing them via the URL redactor.
func redactURLsImpl(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		// Detect a scheme prefix.
		schemeIdx := findSchemeStart(s, i)
		if schemeIdx < 0 {
			b.WriteString(s[i:])
			break
		}
		// Emit everything up to the scheme.
		b.WriteString(s[i:schemeIdx])
		// Find the end of the URL.
		end := schemeIdx
		for end < len(s) {
			c := s[end]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '"' ||
				c == '\'' || c == ')' || c == '<' || c == '>' || c == ',' {
				break
			}
			end++
		}
		urlCandidate := s[schemeIdx:end]
		b.WriteString(redact.RedactURL(urlCandidate))
		i = end
	}
	return b.String()
}

// findSchemeStart returns the index of a "scheme://" prefix starting at or
// after `start`. The scheme must be a contiguous run of letters or "+.-".
func findSchemeStart(s string, start int) int {
	for i := start; i+3 <= len(s); i++ {
		// Scheme must start with a letter.
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			continue
		}
		// Find the end of the scheme.
		j := i + 1
		for j < len(s) {
			cj := s[j]
			if (cj >= 'a' && cj <= 'z') || (cj >= 'A' && cj <= 'Z') ||
				(cj >= '0' && cj <= '9') || cj == '+' || cj == '-' || cj == '.' {
				j++
				continue
			}
			break
		}
		if j <= i {
			continue
		}
		// Require "://".
		if j+3 > len(s) {
			return -1
		}
		if s[j] != ':' || s[j+1] != '/' || s[j+2] != '/' {
			continue
		}
		return i
	}
	return -1
}
