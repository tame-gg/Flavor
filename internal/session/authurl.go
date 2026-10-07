package session

import (
	"net/url"
	"strings"
)

func parseAuthURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidAuthURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", ErrInvalidAuthURL
	}
	switch u.Scheme {
	case "https", "http":
	default:
		return "", ErrInvalidAuthURL
	}
	if u.User != nil {
		return "", ErrInvalidAuthURL
	}
	return u.String(), nil
}

func authHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
