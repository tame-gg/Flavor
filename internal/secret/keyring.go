package secret

import (
	"errors"

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
	if errors.Is(err, ErrLocked) {
		return ErrLocked
	}
	return ErrUnavailable
}
