package handler

import (
	"net/http"

	"do-ddns/server/app"
)

// Ping returns HTTP 204 No Content.
// It is used to check whether the service is running.
func Ping(e *app.Env, w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}
