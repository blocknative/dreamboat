package relay

import (
	"io"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gorilla/mux"
	"github.com/lthibault/log"
)

func withDrainBody() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			defer io.Copy(io.Discard, r.Body)

			next.ServeHTTP(w, r)
		})
	}
}

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

			t0 := time.Now()
			rw := wrapResponseWriter(w)

			l = l.With(log.F{
				"method": r.Method,
				"path":   r.URL.EscapedPath(),
			})

			next.ServeHTTP(rw, r)

			l.With(log.F{
				"status":   rw.Status,
				"duration": time.Since(t0).Seconds(),
			}).Info("request handled")
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	Status int
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w}
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.Status == 0 {
		rw.Status = code
		rw.ResponseWriter.WriteHeader(code)
	}
}
