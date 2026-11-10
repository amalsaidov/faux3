package sigv4

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/change-fabric/faux3/internal/s3err"
)

// AWS documentation example credentials.
const (
	docAK     = "AKIAIOSFODNN7EXAMPLE"
	docSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
)

var docTime = time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

func docVerifier() *Verifier {
	return &Verifier{AccessKeyID: docAK, SecretAccessKey: docSecret, Region: "us-east-1",
		ClockSkew: 15 * time.Minute, Now: func() time.Time { return docTime }}
}

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

var testSigner = Signer{AccessKeyID: "faux3", SecretAccessKey: "faux3", Region: "us-east-1"}

func testVerifier() *Verifier {
	return &Verifier{AccessKeyID: "faux3", SecretAccessKey: "faux3", Region: "us-east-1",
		ClockSkew: 15 * time.Minute, Now: func() time.Time { return t0 }}
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func newReq(method, target string, body []byte) *http.Request {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	r := httptest.NewRequest(method, target, rd)
	r.Host = "localhost:9000"
	return r
}

func wantCode(t *testing.T, err error, code string) *s3err.Error {
	t.Helper()
	var e *s3err.Error
	if !errors.As(err, &e) {
		t.Fatalf("want %s, got %v", code, err)
	}
	if e.Code != code {
		t.Fatalf("want %s, got %s (%s)", code, e.Code, e.Message)
	}
	return e
}

// chunkSpec configures encodeChunked.
type chunkSpec struct {
	key       []byte
	amzDate   string
	scope     string
	seed      string
	signed    bool
	chunkSize int
	trailers  [][2]string // name, value
	signTrail bool
}

// encodeChunked frames data as aws-chunked the way the SDKs do.
func encodeChunked(s chunkSpec, data []byte) []byte {
	var b bytes.Buffer
	prev := s.seed
	write := func(chunk []byte) {
		fmt.Fprintf(&b, "%x", len(chunk))
		if s.signed {
			prev = ChunkSignature(s.key, s.amzDate, s.scope, prev, chunk)
			b.WriteString(";chunk-signature=" + prev)
		}
		b.WriteString("\r\n")
		b.Write(chunk)
		if len(chunk) > 0 {
			b.WriteString("\r\n")
		}
	}
	for off := 0; off < len(data); off += s.chunkSize {
		end := min(off+s.chunkSize, len(data))
		write(data[off:end])
	}
	write(nil)
	if len(s.trailers) == 0 {
		b.WriteString("\r\n")
		return b.Bytes()
	}
	var lines strings.Builder
	for _, tr := range s.trailers {
		b.WriteString(tr[0] + ":" + tr[1] + "\r\n")
		lines.WriteString(tr[0] + ":" + tr[1] + "\n")
	}
	if s.signTrail {
		b.WriteString("x-amz-trailer-signature:" + TrailerSignature(s.key, s.amzDate, s.scope, prev, lines.String()) + "\r\n")
	}
	b.WriteString("\r\n")
	return b.Bytes()
}
