package sigv4

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/change-fabric/faux3/internal/s3err"
)

// MaxPresignExpires is the longest presign lifetime S3 accepts: 7 days.
const MaxPresignExpires = 604800

func queryErr(msg string) error {
	return s3err.New("AuthorizationQueryParametersError", msg)
}

func (v *Verifier) verifyQuery(r *http.Request) (*Verified, error) {
	q := r.URL.Query()
	alg := q.Get("X-Amz-Algorithm")
	if alg == "AWS4-ECDSA-P256-SHA256" {
		return nil, s3err.New("NotImplemented", "SigV4a (AWS4-ECDSA-P256-SHA256) is not supported")
	}
	if alg != Algorithm {
		return nil, queryErr("X-Amz-Algorithm only supports \"" + Algorithm + "\"")
	}
	for _, k := range []string{"X-Amz-Credential", "X-Amz-Date", "X-Amz-Expires", "X-Amz-SignedHeaders", "X-Amz-Signature"} {
		if q.Get(k) == "" {
			return nil, queryErr("Query-string authentication version 4 requires the X-Amz-Algorithm, X-Amz-Credential, X-Amz-Signature, X-Amz-Date, X-Amz-SignedHeaders, and X-Amz-Expires parameters.")
		}
	}
	if tok := q.Get("X-Amz-Security-Token"); tok != "" {
		return nil, s3err.New("InvalidAccessKeyId", "faux3 does not issue session credentials; X-Amz-Security-Token must be empty.")
	}
	cred, ok := parseCredential(q.Get("X-Amz-Credential"))
	if !ok {
		return nil, queryErr("Error parsing the X-Amz-Credential parameter; it must be <key>/<date>/<region>/s3/aws4_request")
	}
	amzDate := q.Get("X-Amz-Date")
	t, ok := parseAmzDate(amzDate)
	if !ok {
		return nil, queryErr("X-Amz-Date must be in the ISO8601 Long Format \"yyyyMMdd'T'HHmmss'Z'\"")
	}
	expires, err := strconv.Atoi(q.Get("X-Amz-Expires"))
	if err != nil || expires < 1 || expires > MaxPresignExpires {
		return nil, queryErr("X-Amz-Expires must be between 1 and 604800 seconds")
	}
	sig := q.Get("X-Amz-Signature")
	if !isHex64(sig) {
		return nil, queryErr("X-Amz-Signature must be 64 hex characters")
	}
	if err := v.checkScope(cred, amzDate, "AuthorizationQueryParametersError", "Error parsing the X-Amz-Credential parameter; "); err != nil {
		return nil, err
	}
	signed := strings.Split(q.Get("X-Amz-SignedHeaders"), ";")
	if !contains(signed, "host") {
		return nil, queryErr("X-Amz-SignedHeaders must include host")
	}
	now := v.now()
	if t.Sub(now) > v.ClockSkew {
		return nil, s3err.New("AccessDenied", "Request is not valid yet")
	}
	if now.After(t.Add(time.Duration(expires) * time.Second)) {
		return nil, s3err.New("AccessDenied", "Request has expired").
			With("X-Amz-Expires", strconv.Itoa(expires)).
			With("Expires", t.Add(time.Duration(expires)*time.Second).UTC().Format(time.RFC3339)).
			With("ServerTime", now.UTC().Format(time.RFC3339))
	}

	payload := UnsignedPayload
	mode := PayloadUnsigned
	if contains(signed, "x-amz-content-sha256") {
		payload = r.Header.Get("X-Amz-Content-Sha256")
		m, err := payloadMode(payload)
		if err != nil {
			return nil, err
		}
		if m != PayloadSHA256 && m != PayloadUnsigned {
			return nil, s3err.New("InvalidArgument", "streaming payloads are not supported with presigned URLs")
		}
		mode = m
	}
	creq := CanonicalRequest(r.Method, CanonicalURI(r.URL.EscapedPath()), CanonicalQuery(r.URL.RawQuery, "X-Amz-Signature"),
		CanonicalHeaders(r, signed), signed, payload)
	vf, err := v.check(cred, amzDate, creq, sig)
	if err != nil {
		return nil, err
	}
	vf.Presigned = true
	vf.Mode = mode
	if mode == PayloadSHA256 {
		vf.ContentSHA256 = strings.ToLower(payload)
	}
	for k, vals := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-checksum-") || lk == "x-amz-sdk-checksum-algorithm" {
			if vf.Hoisted == nil {
				vf.Hoisted = map[string]string{}
			}
			vf.Hoisted[lk] = vals[0]
		}
	}
	return vf, nil
}
