package domain

import (
	"crypto/rand"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/oklog/ulid/v2"
)

type WorkspaceID string

var (
	ErrInvalidWorkspaceID   = errors.New("invalid workspace id")
	ErrInvalidWorkspaceName = errors.New("invalid workspace name")
	ErrInvalidDescription   = errors.New("invalid workspace description")
)

type Workspace struct {
	ID          WorkspaceID
	Name        string
	Description string
	NetworkIDs  []NetworkID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewWorkspaceID() WorkspaceID {
	return WorkspaceID(ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulid.Monotonic(rand.Reader, 0)).String())
}

func ParseWorkspaceID(s string) (WorkspaceID, error) {
	if _, err := ParseNetworkID(s); err != nil {
		return "", ErrInvalidWorkspaceID
	}
	return WorkspaceID(s), nil
}

func ValidateWorkspaceName(name string) error {
	if name != strings.TrimSpace(name) || name == "" || len(name) > 64 || hasControl(name) {
		return ErrInvalidWorkspaceName
	}
	return nil
}

func ValidateWorkspaceDescription(d string) error {
	if len(d) > 256 || hasControl(d) {
		return ErrInvalidDescription
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
