package domain

import "errors"

var (
	ErrForbidden  = errors.New("forbidden")
	ErrNotFound   = errors.New("not found")
	ErrValidation = errors.New("validation error")
)
