package a2s

import (
	"errors"
	"fmt"
)

var (
	ErrChallengeRequired   = errors.New("challenge required")
	ErrTimeout             = errors.New("request timeout")
	ErrInvalidResponse     = errors.New("invalid response")
	ErrNotConnected        = errors.New("not connected to server")
	ErrTooManyRetries      = errors.New("too many retries")
	ErrUnsupportedFeature  = errors.New("unsupported feature")
	ErrShortResponse       = errors.New("response too short")
)

type ProtocolError struct {
	Expected byte
	Actual   byte
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("unexpected response type: 0x%X, expected: 0x%X", e.Actual, e.Expected)
}