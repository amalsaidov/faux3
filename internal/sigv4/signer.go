package sigv4

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Signer signs requests the way the AWS SDKs do. faux3 uses it in tests
// as the reference client; it is exported so API tests can reuse it.
type Signer struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

// SignedHeaderNames lists host plus every header present on r, lowercased
// and sorted, excluding Authorization and User-Agent.
func SignedHeaderNames(r *http.Request) []string {
	names := []string{"host"}
	for k := range r.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "user-agent" || lk == "host" {
			continue
		}
		names = append(names, lk)
	}
	sort.Strings(names)
	return names
}

// SignHeader sets x-amz-date and x-amz-content-sha256 and an Authorization
// header covering every header on r. It returns the seed signature.
func (s Signer) SignHeader(r *http.Request, t time.Time, payloadHash string) string {
	amzDate := t.UTC().Format(TimeFormat)
	r.Header.Set("X-Amz-Date", amzDate)
	r.Header.Set("X-Amz-Content-Sha256", payloadHash)
	r.Header.Del("Authorization")
	signed := SignedHeaderNames(r)
	date := amzDate[:8]
	scope := Scope(date, s.Region)
	creq := CanonicalRequest(r.Method, CanonicalURI(r.URL.EscapedPath()), CanonicalQuery(r.URL.RawQuery, ""),
		CanonicalHeaders(r, signed), signed, payloadHash)
	sig := Sign(SigningKey(s.SecretAccessKey, date, s.Region), StringToSign(amzDate, scope, creq))
	r.Header.Set("Authorization", Algorithm+" Credential="+s.AccessKeyID+"/"+scope+
		", SignedHeaders="+strings.Join(signed, ";")+", Signature="+sig)
	return sig
}

// Presign adds X-Amz-* query parameters signing only host, with an
// UNSIGNED-PAYLOAD body, as the SDK presigners do.
func (s Signer) Presign(r *http.Request, t time.Time, expires time.Duration) {
	amzDate := t.UTC().Format(TimeFormat)
	date := amzDate[:8]
	scope := Scope(date, s.Region)
	add := "X-Amz-Algorithm=" + Algorithm +
		"&X-Amz-Credential=" + URIEncode(s.AccessKeyID+"/"+scope, true) +
		"&X-Amz-Date=" + amzDate +
		"&X-Amz-Expires=" + strconv.Itoa(int(expires/time.Second)) +
		"&X-Amz-SignedHeaders=host"
	if r.URL.RawQuery != "" {
		r.URL.RawQuery += "&" + add
	} else {
		r.URL.RawQuery = add
	}
	signed := []string{"host"}
	creq := CanonicalRequest(r.Method, CanonicalURI(r.URL.EscapedPath()), CanonicalQuery(r.URL.RawQuery, ""),
		CanonicalHeaders(r, signed), signed, UnsignedPayload)
	sig := Sign(SigningKey(s.SecretAccessKey, date, s.Region), StringToSign(amzDate, scope, creq))
	r.URL.RawQuery += "&X-Amz-Signature=" + sig
	r.RequestURI = r.URL.RequestURI()
}
