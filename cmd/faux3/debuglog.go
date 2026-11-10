package main

import (
	"log/slog"
	"net/http"
	"strings"
)

// wireHeaders are the request headers logged at debug level so the payload
// mode each client uses is visible on the wire.
var wireHeaders = []string{
	"x-amz-content-sha256",
	"content-encoding",
	"content-length",
	"transfer-encoding",
	"x-amz-trailer",
	"x-amz-decoded-content-length",
	"x-amz-sdk-checksum-algorithm",
	"content-md5",
	"x-amz-checksum-mode",
}

// debugLog logs request wire details when debug logging is enabled.
func debugLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if log.Enabled(r.Context(), slog.LevelDebug) && r.URL.Path != "/-/healthz" {
			log.Debug("request", wireAttrs(r)...)
		}
		next.ServeHTTP(w, r)
	})
}

func wireAttrs(r *http.Request) []any {
	attrs := []any{
		"method", r.Method,
		"path", r.URL.EscapedPath(),
		"query", redactQuery(r.URL.RawQuery),
	}
	if r.ContentLength >= 0 {
		attrs = append(attrs, "content_length_field", r.ContentLength)
	}
	if len(r.TransferEncoding) > 0 {
		attrs = append(attrs, "transfer-encoding", strings.Join(r.TransferEncoding, ","))
	}
	if a := r.Header.Get("Authorization"); a != "" {
		attrs = append(attrs, "authorization", redactAuth(a))
	}
	for _, h := range wireHeaders {
		if v := r.Header.Get(h); v != "" {
			attrs = append(attrs, h, v)
		}
	}
	for k, v := range r.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-checksum-") && lk != "x-amz-checksum-mode" {
			attrs = append(attrs, lk, strings.Join(v, ","))
		}
	}
	return attrs
}

// redactAuth keeps the algorithm, credential scope and SignedHeaders and
// truncates the signature to its first 8 characters.
func redactAuth(a string) string {
	i := strings.Index(a, "Signature=")
	if i < 0 {
		return a
	}
	sig := a[i+len("Signature="):]
	if len(sig) > 8 {
		sig = sig[:8] + "..."
	}
	return a[:i] + "Signature=" + sig
}

func redactQuery(q string) string {
	parts := strings.Split(q, "&")
	for i, p := range parts {
		if strings.HasPrefix(p, "X-Amz-Signature=") && len(p) > len("X-Amz-Signature=")+8 {
			parts[i] = p[:len("X-Amz-Signature=")+8] + "..."
		}
	}
	return strings.Join(parts, "&")
}
