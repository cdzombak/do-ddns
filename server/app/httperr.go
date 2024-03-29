package app

import "net/http"

// HTTPHandlerError represents an error in a HTTP handler.
// It allows attaching an HTTP status code and a publicly-viewable message
type HTTPHandlerError interface {
	error
	GetStatusCode() int
	GetPublicError() string
}

// HandlerError represents an error with an associated HTTP status code.
// It embeds the built-in error interface including Unwrap().
type HandlerError struct {
	StatusCode  int
	Err         error
	PublicError string
}

// HandlerError conforms to the HTTPHandlerError interface.
var _ HTTPHandlerError = &HandlerError{}

// Error allows HandlerError to satisfy the Golang error interface.
func (e HandlerError) Error() string {
	if e.Err == nil && e.PublicError != "" {
		return e.PublicError
	}
	return e.Err.Error()
}

// Unwrap allows HandlerError to satisfy the Golang 1.13 error interface.
func (e HandlerError) Unwrap() error {
	return e.Err
}

// GetStatusCode returns the error's HTTP status code, or 500 if none is set.
func (e HandlerError) GetStatusCode() int {
	if e.StatusCode != 0 {
		return e.StatusCode
	}
	return http.StatusInternalServerError
}

// GetPublicError returns the error's public message, or the HTTP status text for the error's code
// if no public message is set.
func (e HandlerError) GetPublicError() string {
	if len(e.PublicError) > 0 {
		return e.PublicError
	}
	return http.StatusText(e.GetStatusCode())
}
