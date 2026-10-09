package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
)

type Network struct {
	ID           NetworkID
	DisplayName  string
	Provider     ProviderType
	ControlURL   string
	AutoConnect  bool
	NodeHostname string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

var (
	ErrInvalidDisplayName  = errors.New("invalid display name")
	ErrInvalidNodeHostname = errors.New("invalid node hostname")
	ErrInvalidControlURL   = errors.New("invalid control url")
	ErrInvalidNetwork      = errors.New("invalid network")
)

func (n Network) Validate() error {
	if _, err := ParseNetworkID(string(n.ID)); err != nil {
		return fmt.Errorf("%w: id", ErrInvalidNetwork)
	}
	if err := ValidateDisplayName(n.DisplayName); err != nil {
		return err
	}
	if _, err := ParseProvider(string(n.Provider)); err != nil {
		return err
	}
	if err := ValidateNodeHostname(n.NodeHostname); err != nil {
		return err
	}
	switch n.Provider {
	case ProviderTailscale:
		if n.ControlURL != "" {
			return fmt.Errorf("%w: tailscale control url must be empty", ErrInvalidNetwork)
		}
	case ProviderHeadscale:
		if n.ControlURL == "" {
			return fmt.Errorf("%w: headscale control url required", ErrInvalidNetwork)
		}
	}
	return nil
}

func ValidateDisplayName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return ErrInvalidDisplayName
	}
	for _, r := range name {
		if r == 0 || unicode.IsControl(r) {
			return ErrInvalidDisplayName
		}
	}
	return nil
}

func ValidateNodeHostname(host string) error {
	host = strings.TrimSpace(host)
	if host == "" || len(host) > 63 {
		return ErrInvalidNodeHostname
	}
	for i, r := range host {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' && i > 0 && i < len(host)-1:
		default:
			return ErrInvalidNodeHostname
		}
	}
	return nil
}

func NormalizeControlURL(provider ProviderType, raw string) (string, error) {
	switch provider {
	case ProviderTailscale:
		return "", nil
	case ProviderHeadscale:
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return "", ErrInvalidControlURL
		}
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "", ErrInvalidControlURL
		}
		switch u.Scheme {
		case "https", "http":
		default:
			return "", ErrInvalidControlURL
		}
		u.Fragment = ""
		u.RawQuery = ""
		u.Path = strings.TrimRight(u.Path, "/")
		if u.Path == "/" {
			u.Path = ""
		}
		if u.User != nil {
			return "", ErrInvalidControlURL
		}
		out := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
		out = strings.TrimRight(out, "/")
		return out, nil
	default:
		return "", ErrInvalidProvider
	}
}
