package secret

import "log/slog"

type Secret struct {
	value string
}

func New(value string) Secret {
	return Secret{value: value}
}

func (s Secret) Reveal() string { return s.value }

func (s Secret) String() string { return "[REDACTED]" }

func (s Secret) GoString() string { return "[REDACTED]" }

func (s Secret) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}
