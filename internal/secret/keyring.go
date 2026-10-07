package secret

import (
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

type defaultKeyring struct{}

func (defaultKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (defaultKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (defaultKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

func mapKeyringErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "locked") || strings.Contains(msg, "unlock") {
		return ErrLocked
	}
	return ErrUnavailable
}
