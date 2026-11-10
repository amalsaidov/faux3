package s3api

import (
	"net/http"

	"github.com/change-fabric/faux3/internal/s3err"
)

const corsExposeHeaders = "ETag, x-amz-request-id, x-amz-id-2, Content-Length, Content-Range, " +
	"x-amz-checksum-crc32, x-amz-checksum-crc32c, x-amz-checksum-sha1, x-amz-checksum-sha256, x-amz-checksum-crc64nvme"

// allowOrigin returns the Access-Control-Allow-Origin value for origin,
// or "" when CORS is disabled or the origin is not allowed.
func (s *Server) allowOrigin(origin string) string {
	if origin == "" {
		return ""
	}
	for _, o := range s.CORSOrigins {
		if o == "*" {
			return "*"
		}
		if o == origin {
			return origin
		}
	}
	return ""
}

// preflight answers an unauthenticated OPTIONS request.
func (s *Server) preflight(w http.ResponseWriter, r *http.Request) {
	if len(s.CORSOrigins) == 0 {
		writeError(w, r, s3err.New("AccessDenied", "CORS is not enabled; set FAUX3_CORS_ORIGINS"), "", "")
		return
	}
	allow := s.allowOrigin(r.Header.Get("Origin"))
	method := r.Header.Get("Access-Control-Request-Method")
	if allow == "" || method == "" {
		writeError(w, r, s3err.New("AccessDenied", "CORSResponse: This CORS request is not allowed. This is usually because the evalution of Origin, request method / Access-Control-Request-Method or Access-Control-Request-Headers are not whitelisted by the resource's CORS spec."), "", "")
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", allow)
	h.Set("Access-Control-Allow-Methods", "GET, PUT, HEAD, DELETE, POST")
	if rh := r.Header.Get("Access-Control-Request-Headers"); rh != "" {
		h.Set("Access-Control-Allow-Headers", rh)
	}
	h.Set("Access-Control-Max-Age", "3000")
	h.Add("Vary", "Origin")
	w.WriteHeader(http.StatusOK)
}

// addCORSHeaders decorates an actual (non-preflight) response.
func (s *Server) addCORSHeaders(w http.ResponseWriter, r *http.Request) {
	allow := s.allowOrigin(r.Header.Get("Origin"))
	if allow == "" {
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", allow)
	h.Set("Access-Control-Expose-Headers", corsExposeHeaders)
	h.Add("Vary", "Origin")
}
