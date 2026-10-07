package logging

import (
	"net/url"
	"strings"
)

func SanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Redacted
	}
	safe := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   u.Path,
	}
	return safe.String()
}
