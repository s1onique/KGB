package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactStructuredJSON_SanitizesEmbeddedJSONAndHeaders(t *testing.T) {
	const (
		password = "embedded-password-sentinel"
		apiKey   = "embedded-api-key-sentinel"
		session  = "embedded-session-sentinel"
		bearer   = "embedded-bearer-sentinel"
	)
	input, err := json.Marshal(map[string]string{
		"embedded": `{"password":"` + password + `","api_key":"` + apiKey + `"}`,
		"headers":  "Authorization: Bearer " + bearer + "\nX-Session-Token: " + session,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	got, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON: %v", err)
	}
	for _, banned := range []string{password, apiKey, session, bearer} {
		if strings.Contains(string(got), banned) {
			t.Errorf("sanitized JSON retains exact sentinel %q", banned)
		}
	}
	for _, want := range []string{
		`\"password\":\"[REDACTED]\"`,
		`\"api_key\":\"[REDACTED]\"`,
		"Authorization: [REDACTED]",
		"X-Session-Token: [REDACTED]",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("sanitized JSON missing %q: %s", want, got)
		}
	}

	second, err := RedactStructuredJSON(got)
	if err != nil {
		t.Fatalf("second redaction: %v", err)
	}
	if string(second) != string(got) {
		t.Errorf("structured string redaction is not idempotent\nfirst:  %s\nsecond: %s", got, second)
	}
}
