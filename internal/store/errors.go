package store

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrDuplicateLabel = errors.New("duplicate label")
	ErrBadDimension   = errors.New("bad dimension")
	ErrNonFiniteValue = errors.New("non-finite value in data")
	ErrZeroVector     = errors.New("zero vector (norm is zero)")
	ErrEmptyLabel     = errors.New("empty label")
	ErrLabelTooLong   = errors.New("label too long")
	ErrStoreEmpty     = errors.New("store is empty")
	ErrKOutOfRange    = errors.New("k out of range")
	// ErrNoChanges signals that Save had nothing new to persist since the
	// last successful Save / Load. Callers should treat this as a no-op
	// success, not a failure.
	ErrNoChanges = errors.New("store has no changes since last snapshot")
)
