package sigv4

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Presigned GET example from the AWS S3 SigV4 query auth documentation.
func TestDocPresignExample(t *testing.T) {
	target := "/test.txt?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request" +
		"&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host" +
		"&X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"
	r := newReq("GET", target, nil)
	r.Host = "examplebucket.s3.amazonaws.com"
	vf, err := docVerifier().Verify(r)
	if err != nil {
		t.Fatal(err)
	}
	if !vf.Presigned || vf.Mode != PayloadUnsigned {
		t.Fatalf("%+v", vf)
	}
	v := docVerifier()
	v.Now = func() time.Time { return docTime.Add(86401 * time.Second) }
	_, err = v.Verify(r)
	if e := wantCode(t, err, "AccessDenied"); e.Message != "Request has expired" {
		t.Fatal(e.Message)
	}
}

func presigned(method, target string, at time.Time, exp time.Duration) *http.Request {
	r := newReq(method, target, nil)
	testSigner.Presign(r, at, exp)
	return r
}

func TestPresignRoundTrip(t *testing.T) {
	for _, tg := range []string{"/b/k", "/b/a%20b%2Bc", "/b/k?response-content-type=text%2Fplain", "/b//x/../y"} {
		r := presigned("GET", tg, t0, time.Hour)
		if _, err := testVerifier().Verify(r); err != nil {
			t.Errorf("%s: %v", tg, err)
		}
	}
}

func TestPresignHoistedChecksum(t *testing.T) {
	r := presigned("PUT", "/b/k?x-amz-sdk-checksum-algorithm=CRC32&x-amz-checksum-crc32=AAAAAA%3D%3D", t0, time.Hour)
	vf, err := testVerifier().Verify(r)
	if err != nil {
		t.Fatal(err)
	}
	if vf.Hoisted["x-amz-checksum-crc32"] != "AAAAAA==" || vf.Hoisted["x-amz-sdk-checksum-algorithm"] != "CRC32" {
		t.Fatalf("hoisted %v", vf.Hoisted)
	}
}

func TestPresignNegative(t *testing.T) {
	edit := func(r *http.Request, f func(q url.Values)) *http.Request {
		q := r.URL.Query()
		f(q)
		r.URL.RawQuery = q.Encode()
		return r
	}
	cases := []struct {
		name string
		req  func() *http.Request
		code string
		msg  string
	}{
		{"expired", func() *http.Request { return presigned("GET", "/b/k", t0.Add(-2*time.Second), time.Second) }, "AccessDenied", "Request has expired"},
		{"not yet valid", func() *http.Request { return presigned("GET", "/b/k", t0.Add(20*time.Minute), time.Hour) }, "AccessDenied", "Request is not valid yet"},
		{"expires too long", func() *http.Request { return presigned("GET", "/b/k", t0, 604801*time.Second) }, "AuthorizationQueryParametersError", ""},
		{"expires zero", func() *http.Request {
			return edit(presigned("GET", "/b/k", t0, time.Hour), func(q url.Values) { q.Set("X-Amz-Expires", "0") })
		}, "AuthorizationQueryParametersError", ""},
		{"missing date", func() *http.Request {
			return edit(presigned("GET", "/b/k", t0, time.Hour), func(q url.Values) { q.Del("X-Amz-Date") })
		}, "AuthorizationQueryParametersError", ""},
		{"bad algorithm", func() *http.Request {
			return edit(presigned("GET", "/b/k", t0, time.Hour), func(q url.Values) { q.Set("X-Amz-Algorithm", "HMAC") })
		}, "AuthorizationQueryParametersError", ""},
		{"sigv4a", func() *http.Request {
			return edit(presigned("GET", "/b/k", t0, time.Hour), func(q url.Values) { q.Set("X-Amz-Algorithm", "AWS4-ECDSA-P256-SHA256") })
		}, "NotImplemented", ""},
		{"security token", func() *http.Request {
			return edit(presigned("GET", "/b/k", t0, time.Hour), func(q url.Values) { q.Set("X-Amz-Security-Token", "tok") })
		}, "InvalidAccessKeyId", ""},
		{"unknown key", func() *http.Request {
			s := testSigner
			s.AccessKeyID = "nobody"
			r := newReq("GET", "/b/k", nil)
			s.Presign(r, t0, time.Hour)
			return r
		}, "InvalidAccessKeyId", ""},
		{"wrong region", func() *http.Request {
			s := testSigner
			s.Region = "eu-west-1"
			r := newReq("GET", "/b/k", nil)
			s.Presign(r, t0, time.Hour)
			return r
		}, "AuthorizationQueryParametersError", ""},
		{"tampered expires", func() *http.Request {
			r := presigned("GET", "/b/k", t0, time.Hour)
			r.URL.RawQuery = strings.Replace(r.URL.RawQuery, "X-Amz-Expires=3600", "X-Amz-Expires=7200", 1)
			return r
		}, "SignatureDoesNotMatch", ""},
		{"tampered checksum param", func() *http.Request {
			r := presigned("PUT", "/b/k?x-amz-checksum-crc32=AAAAAA%3D%3D", t0, time.Hour)
			r.URL.RawQuery = strings.Replace(r.URL.RawQuery, "AAAAAA", "BBBBBB", 1)
			return r
		}, "SignatureDoesNotMatch", ""},
		{"wrong secret", func() *http.Request {
			s := testSigner
			s.SecretAccessKey = "nope"
			r := newReq("GET", "/b/k", nil)
			s.Presign(r, t0, time.Hour)
			return r
		}, "SignatureDoesNotMatch", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := testVerifier().Verify(c.req())
			e := wantCode(t, err, c.code)
			if c.msg != "" && e.Message != c.msg {
				t.Fatalf("message %q", e.Message)
			}
		})
	}
}

func TestPresignEmptySecurityTokenAccepted(t *testing.T) {
	r := newReq("GET", "/b/k?X-Amz-Security-Token=", nil)
	testSigner.Presign(r, t0, time.Hour)
	if _, err := testVerifier().Verify(r); err != nil {
		t.Fatal(err)
	}
}
