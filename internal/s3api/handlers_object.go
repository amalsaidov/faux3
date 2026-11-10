package s3api

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/change-fabric/faux3/internal/checksum"
	"github.com/change-fabric/faux3/internal/s3err"
	"github.com/change-fabric/faux3/internal/sigv4"
	"github.com/change-fabric/faux3/internal/store"
)

func (s *Server) putObject(w http.ResponseWriter, r *http.Request, req *request) {
	if !req.ver.Mode.Streaming() && r.ContentLength < 0 {
		writeError(w, r, s3err.New("MissingContentLength", "You must provide the Content-Length HTTP header."), req.bucket, req.key)
		return
	}
	sums, err := checksum.NewSet(r.Header, req.ver.Hoisted)
	if err != nil {
		writeError(w, r, err, req.bucket, req.key)
		return
	}
	p, err := req.ver.Payload(r)
	if err != nil {
		writeError(w, r, err, req.bucket, req.key)
		return
	}
	meta := &store.Meta{
		ContentType:        r.Header.Get("Content-Type"),
		ContentEncoding:    sigv4.StripAWSChunked(r.Header.Get("Content-Encoding")),
		ContentDisposition: r.Header.Get("Content-Disposition"),
		CacheControl:       r.Header.Get("Cache-Control"),
		ContentLanguage:    r.Header.Get("Content-Language"),
		Expires:            r.Header.Get("Expires"),
		UserMetadata:       userMetadata(r.Header),
	}
	stored, err := s.Store.PutObject(r.Context(), req.bucket, req.key, store.PutInput{
		Body:          io.TeeReader(p, sums.Writer()),
		ContentLength: p.DecodedLength,
		Meta:          meta,
		IfNoneMatch:   r.Header.Get("If-None-Match") == "*",
		Verify: func() error {
			if err := p.Verify(); err != nil {
				return err
			}
			if err := sums.Verify(p.Trailers()); err != nil {
				return err
			}
			if computed := sums.Sums(); len(computed) > 0 {
				meta.Checksums = map[string]string{}
				for a, v := range computed {
					meta.Checksums[string(a)] = v
				}
			}
			return nil
		},
	})
	if err != nil {
		writeError(w, r, err, req.bucket, req.key)
		return
	}
	w.Header().Set("ETag", quoteETag(stored.ETag))
	for a, v := range stored.Checksums {
		w.Header().Set(checksum.HeaderName(checksum.Algorithm(a)), v)
	}
	w.WriteHeader(http.StatusOK)
}

func userMetadata(h http.Header) map[string]string {
	var out map[string]string
	for k, v := range h {
		lk := strings.ToLower(k)
		if name, ok := strings.CutPrefix(lk, "x-amz-meta-"); ok && name != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[name] = strings.Join(v, ",")
		}
	}
	return out
}

func quoteETag(etag string) string { return `"` + etag + `"` }

// responseOverrides maps response-* query params to the headers they set.
var responseOverrides = map[string]string{
	"response-content-type":        "Content-Type",
	"response-content-language":    "Content-Language",
	"response-expires":             "Expires",
	"response-cache-control":       "Cache-Control",
	"response-content-disposition": "Content-Disposition",
	"response-content-encoding":    "Content-Encoding",
}

// getObject serves GetObject and HeadObject through http.ServeContent,
// after answering the cases where S3 differs from net/http: XML errors
// for unsatisfiable ranges and failed preconditions, and a full 200 for
// multi-range requests.
func (s *Server) getObject(w http.ResponseWriter, r *http.Request, req *request) {
	f, meta, err := s.Store.GetObject(req.bucket, req.key)
	if err != nil {
		writeError(w, r, err, req.bucket, req.key)
		return
	}
	defer f.Close()

	etag := quoteETag(meta.ETag)
	modtime := meta.LastModified.Truncate(time.Second)
	if err := checkPreconditions(r, etag, modtime); err != nil {
		writeError(w, r, err, req.bucket, req.key)
		return
	}
	ranged, err := checkRange(r, meta.Size)
	if err != nil {
		writeError(w, r, err, req.bucket, req.key)
		return
	}

	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Accept-Ranges", "bytes")
	h.Set("Content-Type", meta.ContentType)
	setIf(h, "Content-Encoding", meta.ContentEncoding)
	setIf(h, "Content-Disposition", meta.ContentDisposition)
	setIf(h, "Cache-Control", meta.CacheControl)
	setIf(h, "Content-Language", meta.ContentLanguage)
	setIf(h, "Expires", meta.Expires)
	for k, v := range meta.UserMetadata {
		h["X-Amz-Meta-"+k] = []string{v}
	}
	for q, hdr := range responseOverrides {
		if req.query.Has(q) {
			h.Set(hdr, req.query.Get(q))
		}
	}
	if !ranged && strings.EqualFold(r.Header.Get("X-Amz-Checksum-Mode"), "ENABLED") && len(meta.Checksums) > 0 {
		for a, v := range meta.Checksums {
			h.Set(checksum.HeaderName(checksum.Algorithm(a)), v)
		}
		h.Set("x-amz-checksum-type", "FULL_OBJECT")
	}
	http.ServeContent(w, r, "", modtime, f)
}

func setIf(h http.Header, name, value string) {
	if value != "" {
		h.Set(name, value)
	}
}

// checkPreconditions evaluates If-Match and If-Unmodified-Since, which
// net/http would answer with a plain-text 412. If-None-Match and
// If-Modified-Since are left to ServeContent (304).
func checkPreconditions(r *http.Request, etag string, modtime time.Time) error {
	failed := func(cond string) error {
		return s3err.New("PreconditionFailed", "At least one of the pre-conditions you specified did not hold").With("Condition", cond)
	}
	if im := r.Header.Get("If-Match"); im != "" {
		for _, t := range strings.Split(im, ",") {
			t = strings.TrimSpace(t)
			if t == "*" || t == etag || `"`+t+`"` == etag {
				return nil
			}
		}
		return failed("If-Match")
	}
	if ius := r.Header.Get("If-Unmodified-Since"); ius != "" {
		if t, err := http.ParseTime(ius); err == nil && modtime.After(t) {
			return failed("If-Unmodified-Since")
		}
	}
	return nil
}

// checkRange validates a Range header against the object size. It
// reports whether a single satisfiable range will be served, drops
// multi-range and malformed headers so the full object is returned with
// 200, and returns InvalidRange for an unsatisfiable single range.
func checkRange(r *http.Request, size int64) (bool, error) {
	rh := r.Header.Get("Range")
	if rh == "" {
		return false, nil
	}
	spec, ok := strings.CutPrefix(rh, "bytes=")
	if !ok || strings.Contains(spec, ",") {
		r.Header.Del("Range")
		return false, nil
	}
	first, last, ok := strings.Cut(strings.TrimSpace(spec), "-")
	if !ok {
		r.Header.Del("Range")
		return false, nil
	}
	invalid := s3err.New("InvalidRange", "The requested range is not satisfiable").
		With("RangeRequested", rh).With("ActualObjectSize", strconv.FormatInt(size, 10))
	if first == "" {
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n < 0 {
			r.Header.Del("Range")
			return false, nil
		}
		if n == 0 || size == 0 {
			return false, invalid
		}
		return true, nil
	}
	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil || start < 0 {
		r.Header.Del("Range")
		return false, nil
	}
	if last != "" {
		end, err := strconv.ParseInt(last, 10, 64)
		if err != nil || end < start {
			r.Header.Del("Range")
			return false, nil
		}
	}
	if start >= size {
		return false, invalid
	}
	return true, nil
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request, req *request) {
	if err := s.Store.DeleteObject(req.bucket, req.key); err != nil {
		writeError(w, r, err, req.bucket, req.key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
