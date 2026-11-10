package sigv4

import (
	"crypto/hmac"
	"net/http"
	"strings"
	"time"

	"github.com/change-fabric/faux3/internal/s3err"
)

// PayloadMode is how the request body is authenticated.
type PayloadMode int

// Payload modes, from the x-amz-content-sha256 value.
const (
	PayloadSHA256 PayloadMode = iota
	PayloadUnsigned
	PayloadStreamingSigned
	PayloadStreamingUnsignedTrailer
	PayloadStreamingSignedTrailer
)

// Streaming reports whether the body is aws-chunked framed.
func (m PayloadMode) Streaming() bool { return m >= PayloadStreamingSigned }

// Verified is the result of a successful signature check.
type Verified struct {
	AccessKeyID string
	AmzDate     string
	Scope       string
	SigningKey  []byte
	// Signature is the request (seed) signature; signed chunks chain from it.
	Signature string
	Mode      PayloadMode
	// ContentSHA256 is the declared hex body hash when Mode is PayloadSHA256.
	ContentSHA256 string
	Presigned     bool
	// Hoisted holds presign query params named x-amz-checksum-* or
	// x-amz-sdk-checksum-algorithm, keyed by lowercased name.
	Hoisted map[string]string
}

// Verifier checks SigV4 header and presigned query authentication against
// one static credential pair and region.
type Verifier struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	ClockSkew       time.Duration
	// Now returns the current time; tests inject a fixed clock.
	Now func() time.Time
}

func (v *Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// Verify authenticates r. Errors are *s3err.Error values.
func (v *Verifier) Verify(r *http.Request) (*Verified, error) {
	auth := r.Header.Get("Authorization")
	q := r.URL.Query()
	hasQuery := q.Has("X-Amz-Algorithm") || q.Has("X-Amz-Credential") || q.Has("X-Amz-Signature")
	switch {
	case auth != "" && hasQuery:
		return nil, s3err.New("InvalidArgument", "Only one auth mechanism allowed; only the X-Amz-Algorithm query parameter or the Authorization header should be specified")
	case auth != "":
		return v.verifyHeader(r, auth)
	case hasQuery:
		return v.verifyQuery(r)
	}
	return nil, s3err.New("AccessDenied", "Access Denied")
}

type authHeader struct {
	credential string
	signed     []string
	signature  string
}

func malformed(msg string) error {
	return s3err.New("AuthorizationHeaderMalformed", "The authorization header is malformed; "+msg)
}

func parseAuthHeader(auth string) (authHeader, error) {
	switch {
	case strings.HasPrefix(auth, "AWS4-ECDSA-P256-SHA256"):
		return authHeader{}, s3err.New("NotImplemented", "SigV4a (AWS4-ECDSA-P256-SHA256) is not supported")
	case strings.HasPrefix(auth, "AWS "):
		return authHeader{}, s3err.New("AccessDenied", "SigV2 is not supported")
	case !strings.HasPrefix(auth, Algorithm+" "):
		return authHeader{}, malformed("the authorization mechanism you have provided is not supported. Please use " + Algorithm + ".")
	}
	var h authHeader
	seen := map[string]bool{}
	for _, part := range strings.Split(strings.TrimPrefix(auth, Algorithm+" "), ",") {
		k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || seen[k] {
			return h, malformed("invalid or repeated component " + quote(k))
		}
		seen[k] = true
		switch k {
		case "Credential":
			h.credential = val
		case "SignedHeaders":
			h.signed = strings.Split(val, ";")
		case "Signature":
			h.signature = val
		default:
			return h, malformed("unknown component " + quote(k))
		}
	}
	if h.credential == "" || len(h.signed) == 0 || h.signature == "" {
		return h, malformed("Credential, SignedHeaders and Signature are required")
	}
	if !isHex64(h.signature) {
		return h, malformed("Signature must be 64 hex characters")
	}
	return h, nil
}

type credential struct {
	accessKey, date, region, service, terminator string
}

func parseCredential(s string) (credential, bool) {
	p := strings.Split(s, "/")
	if len(p) != 5 || p[0] == "" {
		return credential{}, false
	}
	return credential{p[0], p[1], p[2], p[3], p[4]}, true
}

// checkScope validates the credential against the server's key, region,
// and the request date; code is the error code for malformed scopes.
func (v *Verifier) checkScope(c credential, amzDate, code, prefix string) error {
	if c.accessKey != v.AccessKeyID {
		return s3err.New("InvalidAccessKeyId", "The AWS Access Key Id you provided does not exist in our records.").
			With("AWSAccessKeyId", c.accessKey)
	}
	switch {
	case c.date != amzDate[:8]:
		return s3err.New(code, prefix+"the date '"+c.date+"' does not match the request date '"+amzDate[:8]+"'")
	case c.region != v.Region:
		return s3err.New(code, prefix+"the region '"+c.region+"' is wrong; expecting '"+v.Region+"'").
			With("Region", v.Region)
	case c.service != Service:
		return s3err.New(code, prefix+"incorrect service '"+c.service+"'. This endpoint belongs to 's3'.")
	case c.terminator != Terminator:
		return s3err.New(code, prefix+"incorrect terminal '"+c.terminator+"'. This endpoint uses 'aws4_request'.")
	}
	return nil
}

func parseAmzDate(s string) (time.Time, bool) {
	t, err := time.Parse(TimeFormat, s)
	return t, err == nil
}

// payloadMode classifies an x-amz-content-sha256 value.
func payloadMode(v string) (PayloadMode, error) {
	switch {
	case isHex64(v):
		return PayloadSHA256, nil
	case v == UnsignedPayload:
		return PayloadUnsigned, nil
	case v == StreamingSigned:
		return PayloadStreamingSigned, nil
	case v == StreamingSignedTrailer:
		return PayloadStreamingSignedTrailer, nil
	case v == StreamingUnsignedTrailer:
		return PayloadStreamingUnsignedTrailer, nil
	case strings.HasPrefix(v, streamingECDSAPrefix):
		return 0, s3err.New("NotImplemented", "SigV4a streaming payloads are not supported")
	}
	return 0, s3err.New("InvalidArgument", "x-amz-content-sha256 must be UNSIGNED-PAYLOAD, a supported STREAMING- mode, or a valid sha256 value.").
		With("ArgumentName", "x-amz-content-sha256").With("ArgumentValue", v)
}

func (v *Verifier) verifyHeader(r *http.Request, auth string) (*Verified, error) {
	h, err := parseAuthHeader(auth)
	if err != nil {
		return nil, err
	}
	cred, ok := parseCredential(h.credential)
	if !ok {
		return nil, malformed("Credential must be <key>/<date>/<region>/s3/aws4_request")
	}
	amzDate := r.Header.Get("X-Amz-Date")
	if amzDate == "" {
		return nil, s3err.New("AccessDenied", "AWS authentication requires a valid Date or x-amz-date header")
	}
	t, ok := parseAmzDate(amzDate)
	if !ok {
		return nil, s3err.New("AccessDenied", "AWS authentication requires a valid Date or x-amz-date header")
	}
	if err := v.checkScope(cred, amzDate, "AuthorizationHeaderMalformed", "The authorization header is malformed; "); err != nil {
		return nil, err
	}
	if d := v.now().Sub(t); d > v.ClockSkew || d < -v.ClockSkew {
		return nil, s3err.New("RequestTimeTooSkewed", "The difference between the request time and the current time is too large.").
			With("RequestTime", amzDate).With("ServerTime", v.now().UTC().Format(TimeFormat))
	}
	payload := r.Header.Get("X-Amz-Content-Sha256")
	if payload == "" {
		return nil, s3err.New("InvalidRequest", "Missing required header for this request: x-amz-content-sha256")
	}
	mode, err := payloadMode(payload)
	if err != nil {
		return nil, err
	}
	if !contains(h.signed, "host") {
		return nil, malformed("SignedHeaders must include host")
	}
	for _, n := range h.signed {
		if n == "" || n != strings.ToLower(n) {
			return nil, malformed("SignedHeaders must be lowercase header names")
		}
	}
	creq := CanonicalRequest(r.Method, CanonicalURI(r.URL.EscapedPath()), CanonicalQuery(r.URL.RawQuery, ""),
		CanonicalHeaders(r, h.signed), h.signed, payload)
	vf, err := v.check(cred, amzDate, creq, h.signature)
	if err != nil {
		return nil, err
	}
	vf.Mode = mode
	if mode == PayloadSHA256 {
		vf.ContentSHA256 = strings.ToLower(payload)
	}
	return vf, nil
}

// check computes the expected signature and compares in constant time.
func (v *Verifier) check(cred credential, amzDate, creq, provided string) (*Verified, error) {
	scope := Scope(cred.date, cred.region)
	sts := StringToSign(amzDate, scope, creq)
	key := SigningKey(v.SecretAccessKey, cred.date, cred.region)
	want := Sign(key, sts)
	if !hmac.Equal([]byte(want), []byte(strings.ToLower(provided))) {
		return nil, s3err.New("SignatureDoesNotMatch", "The request signature we calculated does not match the signature you provided. Check your key and signing method.").
			With("AWSAccessKeyId", cred.accessKey).
			With("StringToSign", sts).
			With("SignatureProvided", provided).
			With("CanonicalRequest", creq)
	}
	return &Verified{AccessKeyID: cred.accessKey, AmzDate: amzDate, Scope: scope, SigningKey: key, Signature: want}, nil
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func quote(s string) string { return "'" + s + "'" }
