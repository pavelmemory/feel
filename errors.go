package main

import "errors"

type GeneralErrorCause error

var (
	UnsupportedType = errors.New("unsupported type")
	InvalidMapping  = errors.New("invalid mapping")
)

func UnsupportedTypeError(contextCause error) error {
	return Error{GeneralCause: UnsupportedType, ContextCause: contextCause}
}

func InvalidMappingError(contextCause error) error {
	return Error{GeneralCause: InvalidMapping, ContextCause: contextCause}
}

type Error struct {
	GeneralCause GeneralErrorCause
	ContextCause error
}

func (e Error) Error() string {
	switch {
	case e.GeneralCause != nil && e.ContextCause != nil:
		return e.GeneralCause.Error() + ":" + e.ContextCause.Error()
	case e.GeneralCause != nil:
		return e.GeneralCause.Error()
	case e.ContextCause != nil:
		return e.ContextCause.Error()
	}
	return ""
}
