package sigv4

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/change-fabric/faux3/internal/s3err"
)

// Payload is a request body decoded and authenticated per its payload
// mode. Read it to EOF, then call Verify before committing any data.
type Payload struct {
	io.Reader
	// DecodedLength is the body length after aws-chunked decoding, or the
	// Content-Length for plain bodies; -1 when unknown.
	DecodedLength int64
	chunked       *ChunkedReader
	hashed        *sha256Reader
}

// Trailers returns aws-chunked trailing headers (nil for plain bodies).
func (p *Payload) Trailers() map[string]string {
	if p.chunked == nil {
		return nil
	}
	return p.chunked.Trailers()
}

// Verify checks the declared sha256 of a plain body after it is consumed.
// Chunk and trailer signatures are checked while reading.
func (p *Payload) Verify() error {
	if p.hashed != nil {
		return p.hashed.verify()
	}
	return nil
}

// Payload wraps r.Body according to the verified payload mode.
func (v *Verified) Payload(r *http.Request) (*Payload, error) {
	body := r.Body
	if body == nil {
		body = http.NoBody
	}
	if !v.Mode.Streaming() {
		p := &Payload{Reader: body, DecodedLength: r.ContentLength}
		if v.Mode == PayloadSHA256 {
			p.hashed = newSHA256Reader(body, v.ContentSHA256)
			p.Reader = p.hashed
		}
		return p, nil
	}
	dl := r.Header.Get("X-Amz-Decoded-Content-Length")
	if dl == "" {
		return nil, s3err.New("MissingContentLength", "You must provide the x-amz-decoded-content-length HTTP header for aws-chunked uploads.")
	}
	n, err := strconv.ParseInt(dl, 10, 64)
	if err != nil || n < 0 {
		return nil, s3err.New("InvalidArgument", "x-amz-decoded-content-length must be a non-negative integer").
			With("ArgumentName", "x-amz-decoded-content-length").With("ArgumentValue", dl)
	}
	cr, err := NewChunkedReader(body, v, r.Header.Get("X-Amz-Trailer"), n)
	if err != nil {
		return nil, err
	}
	return &Payload{Reader: cr, DecodedLength: n, chunked: cr}, nil
}

// StripAWSChunked removes the aws-chunked token from a Content-Encoding
// value, returning what should be stored (possibly empty).
func StripAWSChunked(contentEncoding string) string {
	var keep []string
	for _, e := range strings.Split(contentEncoding, ",") {
		e = strings.TrimSpace(e)
		if e != "" && !strings.EqualFold(e, "aws-chunked") {
			keep = append(keep, e)
		}
	}
	return strings.Join(keep, ",")
}
