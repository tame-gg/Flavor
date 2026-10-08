package secret

import (
	"errors"
	"fmt"
	"strings"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

type Ref string

var ErrInvalidRef = errors.New("invalid secret ref")

func NetworkRef(id domain.NetworkID, purpose string) (Ref, error) {
	parsed, err := domain.ParseNetworkID(string(id))
	if err != nil {
		return "", fmt.Errorf("%w: network id: %v", ErrInvalidRef, err)
	}
	if err := validatePurpose(purpose); err != nil {
		return "", err
	}
	return Ref("network/" + string(parsed) + "/" + purpose), nil
}

func ParseRef(s string) (Ref, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 || parts[0] != "network" {
		return "", ErrInvalidRef
	}
	if _, err := domain.ParseNetworkID(parts[1]); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidRef, err)
	}
	if err := validatePurpose(parts[2]); err != nil {
		return "", err
	}
	return Ref(s), nil
}

func (r Ref) String() string { return string(r) }

func (r Ref) Account() string { return string(r) }

func validatePurpose(purpose string) error {
	if purpose == "" || len(purpose) > 64 {
		return ErrInvalidRef
	}
	for _, r := range purpose {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return ErrInvalidRef
		}
	}
	return nil
}
