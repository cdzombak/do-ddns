package app

import (
	"log"
	"net/http"
)

// Handler takes a configured Env and a function matching our enhanced handler signature.
type Handler struct {
	E *Env
	H func(e *Env, w http.ResponseWriter, r *http.Request) error
}

// ServeHTTP allows our Handler type to satisfy http.Handler.
// It handles errors returned from the handler, including enhanced errors conforming to HTTPHandlerError.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h.H(h.E, w, r)
	if err != nil {
		switch e := err.(type) {
		case HTTPHandlerError:
			log.Printf("HTTP %d: %s", e.GetStatusCode(), e.Error())
			http.Error(w, e.GetPublicError(), e.GetStatusCode())
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}
}
