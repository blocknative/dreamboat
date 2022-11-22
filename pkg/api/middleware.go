package api

import (
	"net/http"
	"runtime/debug"

	"github.com/gorilla/mux"
	"github.com/lthibault/log"
)

func withAddons(l log.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					w.WriteHeader(http.StatusInternalServerError)

					l = l.WithField("trace", string(debug.Stack()))
					if err, ok := v.(error); ok {
						l = l.WithError(err)
					}

					l.Error("http request panic")
				}
			}()

			w.Header().Set("Content-Type", "application/json")
			next.ServeHTTP(w, r)
		})
	}
}
