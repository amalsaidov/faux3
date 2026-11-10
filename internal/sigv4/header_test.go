package sigv4

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func docAuth(signed, sig string) string {
	return "AWS4-HMAC-SHA256 Credential=" + docAK + "/20130524/us-east-1/s3/aws4_request,SignedHeaders=" +
		signed + ",Signature=" + sig
}

// The four header-auth examples from the AWS S3 SigV4 documentation.
func TestDocHeaderExamples(t *testing.T) {
	put := sha256Hex([]byte("Welcome to Amazon S3."))
	cases := []struct {
		name, method, target string
		headers              map[string]string
		signed, sig          string
	}{
		{"GetObject", "GET", "/test.txt",
			map[string]string{"Range": "bytes=0-9", "X-Amz-Content-Sha256": EmptySHA256},
			"host;range;x-amz-content-sha256;x-amz-date",
			"f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"},
		{"PutObject", "PUT", "/test%24file.text",
			map[string]string{"Date": "Fri, 24 May 2013 00:00:00 GMT", "X-Amz-Storage-Class": "REDUCED_REDUNDANCY", "X-Amz-Content-Sha256": put},
			"date;host;x-amz-content-sha256;x-amz-date;x-amz-storage-class",
			"98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"},
		{"GetBucketLifecycle", "GET", "/?lifecycle",
			map[string]string{"X-Amz-Content-Sha256": EmptySHA256},
			"host;x-amz-content-sha256;x-amz-date",
			"fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543"},
		{"ListObjects", "GET", "/?max-keys=2&prefix=J",
			map[string]string{"X-Amz-Content-Sha256": EmptySHA256},
			"host;x-amz-content-sha256;x-amz-date",
			"34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body []byte
			if c.method == "PUT" {
				body = []byte("Welcome to Amazon S3.")
			}
			r := newReq(c.method, c.target, body)
			r.Host = "examplebucket.s3.amazonaws.com"
			r.Header.Set("X-Amz-Date", "20130524T000000Z")
			for k, v := range c.headers {
				r.Header.Set(k, v)
			}
			r.Header.Set("Authorization", docAuth(c.signed, c.sig))
			vf, err := docVerifier().Verify(r)
			if err != nil {
				t.Fatalf("verify: %v", err)
			}
			if vf.Signature != c.sig || vf.Presigned {
				t.Fatalf("verified %+v", vf)
			}
			p, err := vf.Payload(r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, p); err != nil {
				t.Fatal(err)
			}
			if err := p.Verify(); err != nil {
				t.Fatalf("payload: %v", err)
			}
		})
	}
}

func signedReq(t *testing.T, method, target string, body []byte, mutate func(*http.Request)) *http.Request {
	t.Helper()
	r := newReq(method, target, body)
	if mutate != nil {
		mutate(r)
	}
	ph := EmptySHA256
	if body != nil {
		ph = sha256Hex(body)
	}
	testSigner.SignHeader(r, t0, ph)
	return r
}

func TestReferenceSignerRoundTrip(t *testing.T) {
	targets := []string{
		"/", "/bucket", "/bucket/key", "/bucket/a%20b%2Bc", "/bucket//double/../dots",
		"/bucket/%E2%9C%93/unicode", "/bucket?list-type=2&prefix=a%2Fb&delimiter=%2F",
		"/bucket/a%2Fb", "/bucket/" + strings.Repeat("k", 1024),
	}
	for _, tg := range targets {
		r := signedReq(t, "GET", tg, nil, func(r *http.Request) { r.Header.Set("X-Amz-Meta-Note", "  a   b ") })
		if _, err := testVerifier().Verify(r); err != nil {
			t.Errorf("%s: %v", tg, err)
		}
	}
}

func TestHeaderNegative(t *testing.T) {
	body := []byte("hello")
	cases := []struct {
		name string
		req  func() *http.Request
		code string
	}{
		{"no auth", func() *http.Request { return newReq("GET", "/b", nil) }, "AccessDenied"},
		{"sigv2", func() *http.Request {
			r := newReq("GET", "/b", nil)
			r.Header.Set("Authorization", "AWS faux3:abc")
			return r
		}, "AccessDenied"},
		{"sigv4a", func() *http.Request {
			r := newReq("GET", "/b", nil)
			r.Header.Set("Authorization", "AWS4-ECDSA-P256-SHA256 Credential=x")
			return r
		}, "NotImplemented"},
		{"unknown algorithm", func() *http.Request {
			r := newReq("GET", "/b", nil)
			r.Header.Set("Authorization", "Bearer x")
			return r
		}, "AuthorizationHeaderMalformed"},
		{"missing signature", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			a := r.Header.Get("Authorization")
			r.Header.Set("Authorization", a[:strings.Index(a, ", Signature=")])
			return r
		}, "AuthorizationHeaderMalformed"},
		{"bad credential", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			r.Header.Set("Authorization", strings.Replace(r.Header.Get("Authorization"), "/us-east-1/s3", "/us-east-1", 1))
			return r
		}, "AuthorizationHeaderMalformed"},
		{"unknown access key", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			r.Header.Set("Authorization", strings.Replace(r.Header.Get("Authorization"), "Credential=faux3/", "Credential=nobody/", 1))
			return r
		}, "InvalidAccessKeyId"},
		{"wrong region", func() *http.Request {
			s := testSigner
			s.Region = "eu-west-1"
			r := newReq("GET", "/b", nil)
			s.SignHeader(r, t0, EmptySHA256)
			return r
		}, "AuthorizationHeaderMalformed"},
		{"scope date mismatch", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			r.Header.Set("Authorization", strings.Replace(r.Header.Get("Authorization"), "/20260101/", "/20251231/", 1))
			return r
		}, "AuthorizationHeaderMalformed"},
		{"wrong service", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			r.Header.Set("Authorization", strings.Replace(r.Header.Get("Authorization"), "/s3/", "/iam/", 1))
			return r
		}, "AuthorizationHeaderMalformed"},
		{"missing x-amz-date", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			r.Header.Del("X-Amz-Date")
			return r
		}, "AccessDenied"},
		{"skewed", func() *http.Request {
			r := newReq("GET", "/b", nil)
			testSigner.SignHeader(r, t0.Add(-16*time.Minute), EmptySHA256)
			return r
		}, "RequestTimeTooSkewed"},
		{"future skewed", func() *http.Request {
			r := newReq("GET", "/b", nil)
			testSigner.SignHeader(r, t0.Add(16*time.Minute), EmptySHA256)
			return r
		}, "RequestTimeTooSkewed"},
		{"missing content sha", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			r.Header.Del("X-Amz-Content-Sha256")
			return r
		}, "InvalidRequest"},
		{"bad content sha", func() *http.Request {
			r := newReq("GET", "/b", nil)
			testSigner.SignHeader(r, t0, "nope")
			return r
		}, "InvalidArgument"},
		{"ecdsa streaming", func() *http.Request {
			r := newReq("PUT", "/b/k", nil)
			testSigner.SignHeader(r, t0, "STREAMING-AWS4-ECDSA-P256-SHA256-PAYLOAD")
			return r
		}, "NotImplemented"},
		{"host not signed", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			r.Header.Set("Authorization", strings.Replace(r.Header.Get("Authorization"), "SignedHeaders=host;", "SignedHeaders=", 1))
			return r
		}, "AuthorizationHeaderMalformed"},
		{"wrong secret", func() *http.Request {
			s := testSigner
			s.SecretAccessKey = "wrong"
			r := newReq("GET", "/b", nil)
			s.SignHeader(r, t0, EmptySHA256)
			return r
		}, "SignatureDoesNotMatch"},
		{"tampered path", func() *http.Request {
			r := signedReq(t, "GET", "/b/k", nil, nil)
			r.URL.Path, r.URL.RawPath = "/b/other", ""
			return r
		}, "SignatureDoesNotMatch"},
		{"tampered query", func() *http.Request {
			r := signedReq(t, "GET", "/b?list-type=2&prefix=a", nil, nil)
			r.URL.RawQuery = "list-type=2&prefix=b"
			return r
		}, "SignatureDoesNotMatch"},
		{"tampered signed header", func() *http.Request {
			r := signedReq(t, "PUT", "/b/k", body, func(r *http.Request) { r.Header.Set("X-Amz-Meta-A", "1") })
			r.Header.Set("X-Amz-Meta-A", "2")
			return r
		}, "SignatureDoesNotMatch"},
		{"tampered host", func() *http.Request {
			r := signedReq(t, "GET", "/b", nil, nil)
			r.Host = "localhost:9001"
			return r
		}, "SignatureDoesNotMatch"},
		{"header and query auth", func() *http.Request {
			r := signedReq(t, "GET", "/b?X-Amz-Algorithm=AWS4-HMAC-SHA256", nil, nil)
			return r
		}, "InvalidArgument"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := testVerifier().Verify(c.req())
			e := wantCode(t, err, c.code)
			if c.code == "SignatureDoesNotMatch" {
				names := map[string]bool{}
				for _, f := range e.Fields {
					names[f.Name] = true
				}
				for _, n := range []string{"StringToSign", "CanonicalRequest", "SignatureProvided", "AWSAccessKeyId"} {
					if !names[n] {
						t.Errorf("missing field %s", n)
					}
				}
			}
		})
	}
}

func TestRegionMessage(t *testing.T) {
	s := testSigner
	s.Region = "eu-west-1"
	r := newReq("GET", "/b", nil)
	s.SignHeader(r, t0, EmptySHA256)
	_, err := testVerifier().Verify(r)
	e := wantCode(t, err, "AuthorizationHeaderMalformed")
	if !strings.Contains(e.Message, "the region 'eu-west-1' is wrong; expecting 'us-east-1'") {
		t.Fatalf("message %q", e.Message)
	}
}

func TestPayloadModes(t *testing.T) {
	cases := map[string]PayloadMode{
		UnsignedPayload:          PayloadUnsigned,
		StreamingSigned:          PayloadStreamingSigned,
		StreamingSignedTrailer:   PayloadStreamingSignedTrailer,
		StreamingUnsignedTrailer: PayloadStreamingUnsignedTrailer,
		EmptySHA256:              PayloadSHA256,
	}
	for v, want := range cases {
		r := newReq("PUT", "/b/k", nil)
		testSigner.SignHeader(r, t0, v)
		vf, err := testVerifier().Verify(r)
		if err != nil || vf.Mode != want {
			t.Errorf("%s: mode %v err %v", v, vf, err)
		}
	}
}

func TestSHA256Mismatch(t *testing.T) {
	r := newReq("PUT", "/b/k", []byte("tampered"))
	testSigner.SignHeader(r, t0, sha256Hex([]byte("original")))
	vf, err := testVerifier().Verify(r)
	if err != nil {
		t.Fatal(err)
	}
	p, err := vf.Payload(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, p); err != nil {
		t.Fatal(err)
	}
	wantCode(t, p.Verify(), "XAmzContentSHA256Mismatch")
}
