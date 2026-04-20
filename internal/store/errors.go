package store

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrDuplicateLabel = errors.New("duplicate label")
	ErrBadDimension   = errors.New("bad dimension")
	ErrNonFiniteValue = errors.New("non-finite value in data")
	ErrZeroVector     = errors.New("zero vector (norm is zero)")
	ErrEmptyLabel     = errors.New("empty label")
	ErrStoreEmpty     = errors.New("store is empty")
	ErrKOutOfRange    = errors.New("k out of range")
)
