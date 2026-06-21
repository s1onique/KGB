// Package polling provides typed polling logic for UVB-76 capture netns lab.
//
// This file contains cookie jar handling for session persistence.
// curl-compatible Netscape cookie jar format is supported, including HttpOnly cookies.
package polling

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// CookieDiagnostics provides debug-safe diagnostic information about cookie loading.
type CookieDiagnostics struct {
	CookieJarExists bool
	CookieCount     int
	CookieNames     []string
}

// DiagnoseCookies loads cookie info for debugging without exposing values.
func (c *APIClient) DiagnoseCookies() CookieDiagnostics {
	d := CookieDiagnostics{}

	if c.Cookies == "" {
		return d
	}

	info, err := os.Stat(c.Cookies)
	if err != nil {
		return d // file doesn't exist
	}
	d.CookieJarExists = !info.IsDir()

	data, err := os.ReadFile(c.Cookies)
	if err != nil {
		return d
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 6 {
			d.CookieCount++
			d.CookieNames = append(d.CookieNames, parts[5])
		}
	}

	return d
}

// loadCookies parses a Netscape cookie jar file and adds cookies to the request.
// Supports both regular cookies and HttpOnly cookies (lines starting with #HttpOnly_).
func (c *APIClient) loadCookies(req *http.Request) {
	if c.Cookies == "" {
		return
	}
	data, err := os.ReadFile(c.Cookies)
	if err != nil {
		return
	}
	// Parse Netscape cookie format
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Handle HttpOnly cookies: #HttpOnly_.domain.com
		// These are valid cookies, not comments - strip the prefix
		if strings.HasPrefix(line, "#HttpOnly_") {
			// Strip "#HttpOnly_" prefix to get the domain field
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			// Regular comment lines are ignored
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		// parts[0]=domain, parts[1]=tailmatch, parts[2]=path, parts[3]=secure, parts[4]=expires, parts[5]=name, parts[6]=value
		cookie := &http.Cookie{
			Name:  parts[5],
			Value: parts[6],
			Path:  parts[2],
		}
		req.AddCookie(cookie)
	}
}

// saveCookies extracts cookies from the HTTP response and appends them to the cookie jar.
func (c *APIClient) saveCookies(resp *http.Response) {
	if c.Cookies == "" || len(resp.Cookies()) == 0 {
		return
	}
	// Append cookies to existing file
	f, err := os.OpenFile(c.Cookies, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	for _, cookie := range resp.Cookies() {
		// Write in Netscape cookie format
		fmt.Fprintf(f, ".uvb76\tTRUE\t/\tFALSE\t%d\t%s\t%s\n",
			time.Now().Add(24*time.Hour).Unix(), cookie.Name, cookie.Value)
	}
}
