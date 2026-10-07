package domain

import "errors"

type ProviderType string

const (
	ProviderTailscale ProviderType = "tailscale"
	ProviderHeadscale ProviderType = "headscale"
)

var ErrInvalidProvider = errors.New("invalid provider")

func ParseProvider(s string) (ProviderType, error) {
	switch ProviderType(s) {
	case ProviderTailscale, ProviderHeadscale:
		return ProviderType(s), nil
	default:
		return "", ErrInvalidProvider
	}
}

func (p ProviderType) String() string {
	return string(p)
}
