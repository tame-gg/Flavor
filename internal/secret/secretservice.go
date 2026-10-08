package secret

import (
	"context"
	"errors"
)

type BackendDriver interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type secretServiceStore struct {
	service string
	api     BackendDriver
}

func newSecretServiceStore(service string, api BackendDriver) *secretServiceStore {
	return &secretServiceStore{service: service, api: api}
}

func NewSecretServiceForTest(service string, api BackendDriver) Store {
	return newSecretServiceStore(service, api)
}

func (s *secretServiceStore) Get(ctx context.Context, ref Ref) (Secret, error) {
	if err := ctx.Err(); err != nil {
		return Secret{}, err
	}
	if _, err := ParseRef(string(ref)); err != nil {
		return Secret{}, err
	}
	v, err := s.api.Get(s.service, ref.Account())
	if err != nil {
		return Secret{}, mapKeyringErr(err)
	}
	return New(v), nil
}

func (s *secretServiceStore) Set(ctx context.Context, ref Ref, secret Secret) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := ParseRef(string(ref)); err != nil {
		return err
	}
	return mapKeyringErr(s.api.Set(s.service, ref.Account(), secret.Reveal()))
}

func (s *secretServiceStore) Delete(ctx context.Context, ref Ref) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := ParseRef(string(ref)); err != nil {
		return err
	}
	err := s.api.Delete(s.service, ref.Account())
	if err == nil {
		return nil
	}
	mapped := mapKeyringErr(err)
	if errors.Is(mapped, ErrNotFound) {
		return nil
	}
	return mapped
}

func (s *secretServiceStore) Status(ctx context.Context) Status {
	if err := ctx.Err(); err != nil {
		return Status{Backend: BackendSecretService, State: StateUnavailable, Detail: err.Error()}
	}
	probeRef := "flavor/status-probe"
	_, err := s.api.Get(s.service, probeRef)
	if err == nil {
		return Status{Backend: BackendSecretService, State: StateAvailable}
	}
	mapped := mapKeyringErr(err)
	switch {
	case errors.Is(mapped, ErrNotFound):
		return Status{Backend: BackendSecretService, State: StateAvailable}
	case errors.Is(mapped, ErrLocked):
		return Status{Backend: BackendSecretService, State: StateLocked, Detail: "keyring reported locked/unlock required"}
	default:
		return Status{Backend: BackendSecretService, State: StateUnavailable, Detail: "secret service unreachable or unusable"}
	}
}
