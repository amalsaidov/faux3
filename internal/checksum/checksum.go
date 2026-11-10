// Package checksum computes and verifies the S3 object integrity
// checksums: x-amz-checksum-{crc32,crc32c,crc64nvme,sha1,sha256} and
// Content-MD5. Values are base64 of the big-endian digest.
package checksum

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"hash"
	"hash/crc32"
	"hash/crc64"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/change-fabric/faux3/internal/s3err"
)

// Algorithm is a lowercase checksum algorithm name.
type Algorithm string

// Supported algorithms. MD5 is only reachable through Content-MD5.
const (
	CRC32     Algorithm = "crc32"
	CRC32C    Algorithm = "crc32c"
	CRC64NVME Algorithm = "crc64nvme"
	SHA1      Algorithm = "sha1"
	SHA256    Algorithm = "sha256"
	MD5       Algorithm = "md5"
)

// Algorithms lists the x-amz-checksum-* algorithms.
var Algorithms = []Algorithm{CRC32, CRC32C, CRC64NVME, SHA1, SHA256}

// crc64NVMEPoly is the reflected CRC-64/NVME polynomial.
const crc64NVMEPoly = 0x9a6c9329ac4bc9b5

var (
	castagnoli  = crc32.MakeTable(crc32.Castagnoli)
	crc64NVMETb = crc64.MakeTable(crc64NVMEPoly)
)

// Parse maps a name such as "CRC32" or "crc64nvme" to an Algorithm.
func Parse(name string) (Algorithm, bool) {
	a := Algorithm(strings.ToLower(strings.TrimSpace(name)))
	switch a {
	case CRC32, CRC32C, CRC64NVME, SHA1, SHA256:
		return a, true
	}
	return "", false
}

// New returns a hasher for a.
func New(a Algorithm) hash.Hash {
	switch a {
	case CRC32:
		return crc32.NewIEEE()
	case CRC32C:
		return crc32.New(castagnoli)
	case CRC64NVME:
		return crc64.New(crc64NVMETb)
	case SHA1:
		return sha1.New()
	case SHA256:
		return sha256.New()
	case MD5:
		return md5.New()
	}
	panic("checksum: unknown algorithm " + string(a))
}

// Size returns the digest length in bytes.
func Size(a Algorithm) int { return New(a).Size() }

// Encode returns the base64 form of a digest.
func Encode(sum []byte) string { return base64.StdEncoding.EncodeToString(sum) }

// Sum computes the encoded checksum of b.
func Sum(a Algorithm, b []byte) string {
	h := New(a)
	h.Write(b)
	return Encode(h.Sum(nil))
}

// HeaderName returns the x-amz-checksum-<alg> header name.
func HeaderName(a Algorithm) string { return "x-amz-checksum-" + string(a) }

// fromHeaderName parses x-amz-checksum-<alg>; ok is false for other names
// and for the non-value headers x-amz-checksum-mode and -type.
func fromHeaderName(name string) (alg Algorithm, known, ok bool) {
	name = strings.ToLower(name)
	rest, found := strings.CutPrefix(name, "x-amz-checksum-")
	if !found || rest == "mode" || rest == "type" || rest == "algorithm" {
		return "", false, false
	}
	a, known := Parse(rest)
	return a, known, true
}

func invalidChecksum(msg string) error { return s3err.New("InvalidRequest", msg) }

// Set computes every checksum a request asks for while the body streams
// through Writer, and verifies them with Verify once the body is consumed.
type Set struct {
	hashes   map[Algorithm]hash.Hash
	expected map[Algorithm]string
	// trailerAlg is the algorithm whose expected value arrives in the
	// aws-chunked trailer, if any.
	trailerAlg Algorithm
	writer     io.Writer
}

// NewSet resolves the expected checksums of a request from its headers,
// hoisted presign query params (lowercased keys), and the x-amz-trailer
// header. At most one x-amz-checksum-* value may be supplied.
func NewSet(h http.Header, hoisted map[string]string) (*Set, error) {
	s := &Set{hashes: map[Algorithm]hash.Hash{}, expected: map[Algorithm]string{}}
	var sources int
	addExpected := func(name, value string) error {
		a, known, ok := fromHeaderName(name)
		if !ok {
			return nil
		}
		if !known {
			return invalidChecksum("Unsupported checksum algorithm in " + name)
		}
		sources++
		if !validValue(a, value) {
			return invalidChecksum("Value for " + name + " header is invalid.")
		}
		s.expected[a] = value
		return nil
	}
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if err := addExpected(k, h.Get(k)); err != nil {
			return nil, err
		}
	}
	for k, v := range hoisted {
		if err := addExpected(k, v); err != nil {
			return nil, err
		}
	}
	if tr := strings.TrimSpace(h.Get("X-Amz-Trailer")); tr != "" {
		for _, n := range strings.Split(tr, ",") {
			a, known, ok := fromHeaderName(strings.TrimSpace(n))
			if !ok {
				continue
			}
			if !known {
				return nil, invalidChecksum("Unsupported checksum algorithm in x-amz-trailer " + n)
			}
			sources++
			s.trailerAlg = a
		}
	}
	if sources > 1 {
		return nil, invalidChecksum("Expecting a single x-amz-checksum- header. Multiple checksum Types are not allowed.")
	}
	for a := range s.expected {
		s.hashes[a] = New(a)
	}
	if s.trailerAlg != "" {
		s.hashes[s.trailerAlg] = New(s.trailerAlg)
	}
	sdkAlg := h.Get("X-Amz-Sdk-Checksum-Algorithm")
	if sdkAlg == "" {
		sdkAlg = hoisted["x-amz-sdk-checksum-algorithm"]
	}
	if sdkAlg != "" {
		a, ok := Parse(sdkAlg)
		if !ok {
			return nil, invalidChecksum("Checksum algorithm provided is unsupported. Please try again with any of the valid types: [CRC32, CRC32C, CRC64NVME, SHA1, SHA256]")
		}
		if _, have := s.hashes[a]; !have {
			s.hashes[a] = New(a)
		}
	}
	if md := h.Get("Content-Md5"); md != "" {
		if !validValue(MD5, md) {
			return nil, s3err.New("InvalidDigest", "The Content-MD5 you specified was invalid.")
		}
		s.expected[MD5] = md
		s.hashes[MD5] = New(MD5)
	}
	ws := make([]io.Writer, 0, len(s.hashes))
	for _, hh := range s.hashes {
		ws = append(ws, hh)
	}
	s.writer = io.MultiWriter(ws...)
	return s, nil
}

func validValue(a Algorithm, v string) bool {
	b, err := base64.StdEncoding.DecodeString(v)
	return err == nil && len(b) == Size(a)
}

// Writer receives the decoded body bytes.
func (s *Set) Writer() io.Writer { return s.writer }

// TrailerAlgorithm is the algorithm expected in the trailer, or "".
func (s *Set) TrailerAlgorithm() Algorithm { return s.trailerAlg }

// Verify compares the computed digests with the expected values. trailers
// are the aws-chunked trailing headers (lowercase keys), or nil.
func (s *Set) Verify(trailers map[string]string) error {
	if s.trailerAlg != "" {
		name := HeaderName(s.trailerAlg)
		v, ok := trailers[name]
		if !ok {
			return invalidChecksum("The " + name + " trailer was declared but not sent.")
		}
		if !validValue(s.trailerAlg, v) {
			return invalidChecksum("Value for " + name + " trailing header is invalid.")
		}
		s.expected[s.trailerAlg] = v
	}
	for a, want := range s.expected {
		got := Encode(s.hashes[a].Sum(nil))
		if got == want {
			continue
		}
		if a == MD5 {
			return s3err.New("BadDigest", "The Content-MD5 you specified did not match what we received.")
		}
		return s3err.New("BadDigest", "The "+strings.ToUpper(string(a))+" you specified did not match the calculated checksum.")
	}
	return nil
}

// Sums returns the computed x-amz-checksum-* values (MD5 excluded), keyed
// by algorithm, for storage in object metadata.
func (s *Set) Sums() map[Algorithm]string {
	out := map[Algorithm]string{}
	for a, h := range s.hashes {
		if a != MD5 {
			out[a] = Encode(h.Sum(nil))
		}
	}
	return out
}
