package session

import "git.lunarlabs.dev/flavor/flavor/internal/secret"

type EnrollmentMethod string

const (
	EnrollmentNone    EnrollmentMethod = ""
	EnrollmentAuthKey EnrollmentMethod = "auth_key"
)

type EnrollmentInput struct {
	Method     EnrollmentMethod
	Credential secret.Secret
}

func (e *EnrollmentInput) validate() error {
	if e == nil {
		return nil
	}
	switch e.Method {
	case EnrollmentNone:
		return nil
	case EnrollmentAuthKey:
		if e.Credential.Reveal() == "" {
			return ErrInvalidEnrollment
		}
		return nil
	default:
		return ErrInvalidEnrollment
	}
}
