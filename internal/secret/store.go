package secret

import (
	"context"
	"errors"
)

type Backend string

const (
	BackendSecretService Backend = "secret-service"
	BackendMemory        Backend = "memory"
)

type State string

const (
	StateAvailable   State = "available"
	StateLocked      State = "locked"
	StateUnavailable State = "unavailable"
	StateMemory      State = "memory"
)

type Status struct {
	Backend Backend
	State   State
	Detail  string
}

func (s Status) Persistent() bool {
	return s.Backend == BackendSecretService && s.State == StateAvailable
}

var (
	ErrNotFound    = errors.New("secret not found")
	ErrUnavailable = errors.New("secret store unavailable")
	ErrLocked      = errors.New("secret store locked")
)

type Store interface {
	Get(ctx context.Context, ref Ref) (Secret, error)
	Set(ctx context.Context, ref Ref, secret Secret) error
	Delete(ctx context.Context, ref Ref) error
	Status(ctx context.Context) Status
}

type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeMemory Mode = "memory"
)

type Options struct {
	Mode    Mode
	Service string
}

func Open(ctx context.Context, opts Options) (Store, error) {
	if opts.Service == "" {
		opts.Service = DefaultService
	}
	switch opts.Mode {
	case ModeMemory:
		return NewMemoryStore(), nil
	case ModeAuto, "":
		ss := newSecretServiceStore(opts.Service, defaultKeyring{})
		st := ss.Status(ctx)
		if st.State == StateAvailable {
			return ss, nil
		}
		return NewMemoryStore(), nil
	default:
		return nil, ErrUnavailable
	}
}

const DefaultService = "dev.lunarlabs.flavor"
