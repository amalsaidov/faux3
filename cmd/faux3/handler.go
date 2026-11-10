package main

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"

	"github.com/change-fabric/faux3/internal/s3err"
	"github.com/change-fabric/faux3/internal/sigv4"
)

// newHandler serves the health endpoint unauthenticated and authenticates
// every other request with SigV4 before answering. Until the S3 handlers
// land, authenticated requests get 501 NotImplemented.
func newHandler(log *slog.Logger, v *sigv4.Verifier) http.Handler {
	return debugLog(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/healthz" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, "ok")
			return
		}
		id := requestID()
		w.Header().Set("x-amz-request-id", id)
		w.Header().Set("x-amz-id-2", id)
		_, err := v.Verify(r)
		drainAndLog(r.Context(), log, r)
		if err != nil {
			e := s3err.As(err)
			log.Info("auth rejected", "method", r.Method, "path", r.URL.EscapedPath(), "code", e.Code, "message", e.Message)
			s3err.Write(w, r, e, id)
			return
		}
		s3err.Write(w, r, s3err.New("NotImplemented",
			"A header or operation you provided implies functionality that is not implemented."), id)
	}))
}

func requestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
