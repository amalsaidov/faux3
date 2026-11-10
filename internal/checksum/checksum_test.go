package checksum

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/change-fabric/faux3/internal/s3err"
)

func b64hex(t *testing.T, h string) string {
	t.Helper()
	b, err := hex.DecodeString(h)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func u32(v uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return base64.StdEncoding.EncodeToString(b[:])
}

func u64(v uint64) string {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return base64.StdEncoding.EncodeToString(b[:])
}

func TestVectors(t *testing.T) {
	check := "123456789"
	cases := []struct {
		alg         Algorithm
		check, empt string
	}{
		{CRC32, u32(0xCBF43926), u32(0)},
		{CRC32C, u32(0xE3069283), u32(0)},
		// AWS-published CRC64NVME check value for "123456789".
		{CRC64NVME, u64(0xAE8B14860A799888), u64(0)},
		{SHA1, b64hex(t, "f7c3bc1d808e04732adf679965ccc34ca7ae3441"), b64hex(t, "da39a3ee5e6b4b0d3255bfef95601890afd80709")},
		{SHA256, b64hex(t, "15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225"), b64hex(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")},
		{MD5, b64hex(t, "25f9e794323b453885f5181f1b624d0b"), b64hex(t, "d41d8cd98f00b204e9800998ecf8427e")},
	}
	for _, c := range cases {
		if got := Sum(c.alg, []byte(check)); got != c.check {
			t.Errorf("%s(123456789)=%s want %s", c.alg, got, c.check)
		}
		if got := Sum(c.alg, nil); got != c.empt {
			t.Errorf("%s(empty)=%s want %s", c.alg, got, c.empt)
		}
	}
}

// The crc32 value observed on the wire from the JS SDK
// for the 25-byte capture body.
func TestWireCaptureValues(t *testing.T) {
	if got := Sum(CRC32, []byte("hello faux3 wire capture\n")); got != "X9wasw==" {
		t.Fatalf("crc32 %s", got)
	}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *s3err.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func run(t *testing.T, h http.Header, hoisted map[string]string, body string, trailers map[string]string) (*Set, error) {
	t.Helper()
	s, err := NewSet(h, hoisted)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(s.Writer(), strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	return s, s.Verify(trailers)
}

func hdr(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return h
}

func TestSetSources(t *testing.T) {
	body := "123456789"
	for _, a := range Algorithms {
		t.Run("header/"+string(a), func(t *testing.T) {
			s, err := run(t, hdr(HeaderName(a), Sum(a, []byte(body))), nil, body, nil)
			if err != nil {
				t.Fatal(err)
			}
			if s.Sums()[a] != Sum(a, []byte(body)) {
				t.Fatalf("sums %v", s.Sums())
			}
		})
		t.Run("trailer/"+string(a), func(t *testing.T) {
			// No x-amz-sdk-checksum-algorithm: the CLI shape.
			_, err := run(t, hdr("X-Amz-Trailer", HeaderName(a)), nil, body,
				map[string]string{HeaderName(a): Sum(a, []byte(body))})
			if err != nil {
				t.Fatal(err)
			}
		})
		t.Run("hoisted/"+string(a), func(t *testing.T) {
			_, err := run(t, http.Header{}, map[string]string{HeaderName(a): Sum(a, []byte(body))}, body, nil)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := run(t, hdr("Content-MD5", Sum(MD5, []byte(body))), nil, body, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSDKAlgorithmOnlyComputes(t *testing.T) {
	s, err := run(t, hdr("X-Amz-Sdk-Checksum-Algorithm", "CRC64NVME"), nil, "123456789", nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Sums()[CRC64NVME] != u64(0xAE8B14860A799888) {
		t.Fatalf("sums %v", s.Sums())
	}
}

func TestSetNegative(t *testing.T) {
	body := "123456789"
	cases := []struct {
		name     string
		h        http.Header
		hoisted  map[string]string
		trailers map[string]string
		code     string
	}{
		{"crc32 mismatch", hdr("x-amz-checksum-crc32", Sum(CRC32, []byte("x"))), nil, nil, "BadDigest"},
		{"crc64nvme mismatch", hdr("x-amz-checksum-crc64nvme", Sum(CRC64NVME, []byte("x"))), nil, nil, "BadDigest"},
		{"sha256 mismatch", hdr("x-amz-checksum-sha256", Sum(SHA256, []byte("x"))), nil, nil, "BadDigest"},
		{"md5 mismatch", hdr("Content-MD5", Sum(MD5, []byte("x"))), nil, nil, "BadDigest"},
		{"md5 malformed", hdr("Content-MD5", "not base64!"), nil, nil, "InvalidDigest"},
		{"md5 wrong length", hdr("Content-MD5", "AAAAAA=="), nil, nil, "InvalidDigest"},
		{"checksum malformed", hdr("x-amz-checksum-crc32", "@@@"), nil, nil, "InvalidRequest"},
		{"checksum wrong length", hdr("x-amz-checksum-sha1", "AAAAAA=="), nil, nil, "InvalidRequest"},
		{"unknown algorithm", hdr("x-amz-checksum-xxh3", "AAAAAA=="), nil, nil, "InvalidRequest"},
		{"unknown sdk algorithm", hdr("X-Amz-Sdk-Checksum-Algorithm", "XXH3"), nil, nil, "InvalidRequest"},
		{"two headers", hdr("x-amz-checksum-crc32", Sum(CRC32, []byte(body)), "x-amz-checksum-sha1", Sum(SHA1, []byte(body))), nil, nil, "InvalidRequest"},
		{"header and trailer", hdr("x-amz-checksum-crc32", Sum(CRC32, []byte(body)), "X-Amz-Trailer", "x-amz-checksum-crc32c"), nil, nil, "InvalidRequest"},
		{"header and hoisted", hdr("x-amz-checksum-crc32", Sum(CRC32, []byte(body))), map[string]string{"x-amz-checksum-crc32": Sum(CRC32, []byte(body))}, nil, "InvalidRequest"},
		// A default SDK presign hoists the CRC32 of an empty body.
		{"hoisted empty-body crc32", http.Header{}, map[string]string{"x-amz-checksum-crc32": "AAAAAA=="}, nil, "BadDigest"},
		{"trailer mismatch", hdr("X-Amz-Trailer", "x-amz-checksum-crc32"), nil, map[string]string{"x-amz-checksum-crc32": "AAAAAA=="}, "BadDigest"},
		{"trailer missing", hdr("X-Amz-Trailer", "x-amz-checksum-crc32"), nil, map[string]string{}, "InvalidRequest"},
		{"trailer malformed", hdr("X-Amz-Trailer", "x-amz-checksum-crc32"), nil, map[string]string{"x-amz-checksum-crc32": "zz"}, "InvalidRequest"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := run(t, c.h, c.hoisted, body, c.trailers)
			wantCode(t, err, c.code)
		})
	}
}

func TestModeAndTypeHeadersIgnored(t *testing.T) {
	if _, err := run(t, hdr("x-amz-checksum-mode", "ENABLED", "x-amz-checksum-type", "FULL_OBJECT"), nil, "x", nil); err != nil {
		t.Fatal(err)
	}
}
