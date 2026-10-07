package session

import "errors"

var (
	ErrBusy              = errors.New("session busy")
	ErrAlreadyActive     = errors.New("session already active")
	ErrStopped           = errors.New("session stopped during start")
	ErrInvalidConfig     = errors.New("invalid session config")
	ErrInvalidAuthURL    = errors.New("invalid auth url")
	ErrInvalidEnrollment = errors.New("invalid enrollment")
	ErrUnpreparedEnv     = errors.New("process environment not prepared for tsnet")
	errWatcherEnded      = errors.New("ipn watcher ended")
)
