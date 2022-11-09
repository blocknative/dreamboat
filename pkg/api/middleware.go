package api

import (
	"net/http"
	"runtime/debug"

	"github.com/gorilla/mux"
	"github.com/lthibault/log"
)

func withContentType(ct string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			next.ServeHTTP(w, r)
		})
	}
}

// withLogger logs the incoming HTTP request & its duration.
func withLogger(l log.Logger) mux.MiddlewareFunc {
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

			next.ServeHTTP(w, r)
			//t0 := time.Now()
			/*
				l = l.With(log.F{
					"method": r.Method,
					"path":   r.URL.EscapedPath(),
				})

					l.With(log.F{
						"status":   w.Status,
						"duration": time.Since(t0).Seconds(),
					}).Info("request handled") */
		})
	}
}
