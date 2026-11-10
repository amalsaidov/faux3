package s3api

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/change-fabric/faux3/internal/s3err"
)

// statusWriter records the status, byte count, and error code of a
// response for the access log.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
	code   string
}

func (s *statusWriter) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(p)
	s.bytes += int64(n)
	return n, err
}

func newID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// withMiddleware assigns request and host ids, recovers panics into
// InternalError, and writes one access log line per request.
func (s *Server) withMiddleware(next func(w http.ResponseWriter, r *http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		sw.Header().Set("x-amz-request-id", newID(8))
		sw.Header().Set("x-amz-id-2", newID(24))
		op := ""
		defer func() {
			if p := recover(); p != nil {
				s.log().Error("panic", "err", p, "method", r.Method, "path", r.URL.EscapedPath())
				if sw.status == 0 {
					writeError(sw, r, s3err.New("InternalError", "We encountered an internal error. Please try again."), "", "")
				}
			}
			if r.URL.Path == healthPath {
				return
			}
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			attrs := []any{"method", r.Method, "path", r.URL.EscapedPath(), "status", status,
				"bytes", sw.bytes, "duration_ms", time.Since(start).Milliseconds(), "op", op,
				"request_id", sw.Header().Get("x-amz-request-id")}
			if sw.code != "" {
				attrs = append(attrs, "code", sw.code)
			}
			s.log().Info("request", attrs...)
		}()
		op = next(sw, r)
	})
}

func (s *Server) log() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return s.Log
}
