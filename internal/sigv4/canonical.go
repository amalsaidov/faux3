// Package sigv4 verifies AWS Signature Version 4 requests for S3: header
// auth, presigned query auth, and the aws-chunked streaming payload modes.
package sigv4

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Protocol constants.
const (
	Algorithm  = "AWS4-HMAC-SHA256"
	TimeFormat = "20060102T150405Z"
	DateFormat = "20060102"
	Service    = "s3"
	Terminator = "aws4_request"

	EmptySHA256              = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	UnsignedPayload          = "UNSIGNED-PAYLOAD"
	StreamingSigned          = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"
	StreamingSignedTrailer   = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD-TRAILER"
	StreamingUnsignedTrailer = "STREAMING-UNSIGNED-PAYLOAD-TRAILER"
	streamingECDSAPrefix     = "STREAMING-AWS4-ECDSA-P256-SHA256-PAYLOAD"
)

func isUnreserved(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' ||
		c == '-' || c == '.' || c == '_' || c == '~'
}

// URIEncode percent-encodes every byte of s except the unreserved set
// A-Za-z0-9-._~ using uppercase hex. A slash is kept unless encodeSlash.
func URIEncode(s string, encodeSlash bool) string {
	const hexdig = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) || (c == '/' && !encodeSlash) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexdig[c>>4])
		b.WriteByte(hexdig[c&15])
	}
	return b.String()
}

// CanonicalURI decodes the escaped request path once and re-encodes it per
// the S3 rules. It never collapses "//" or "..".
func CanonicalURI(escapedPath string) string {
	if escapedPath == "" {
		return "/"
	}
	dec, err := url.PathUnescape(escapedPath)
	if err != nil {
		dec = escapedPath
	}
	return URIEncode(dec, false)
}

// CanonicalQuery builds the canonical query string from a raw query,
// omitting any parameter whose decoded name equals skip.
func CanonicalQuery(rawQuery, skip string) string {
	type kv struct{ k, v string }
	var pairs []kv
	for _, part := range strings.Split(rawQuery, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		k, v = queryUnescape(k), queryUnescape(v)
		if skip != "" && k == skip {
			continue
		}
		pairs = append(pairs, kv{URIEncode(k, true), URIEncode(v, true)})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
}

func queryUnescape(s string) string {
	d, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return d
}

// headerValues returns the values of a lowercase header name as the server
// received them, including the fields net/http lifts out of the map.
func headerValues(r *http.Request, name string) []string {
	switch name {
	case "host":
		if r.Host != "" {
			return []string{r.Host}
		}
		return []string{r.URL.Host}
	case "content-length":
		if v := r.Header.Values("Content-Length"); len(v) > 0 {
			return v
		}
		if r.ContentLength >= 0 {
			return []string{strconv.FormatInt(r.ContentLength, 10)}
		}
		return nil
	case "transfer-encoding":
		if len(r.TransferEncoding) > 0 {
			return []string{strings.Join(r.TransferEncoding, ",")}
		}
	}
	return r.Header.Values(name)
}

// CanonicalHeaders renders "name:value\n" for each signed header, with
// repeated values joined by commas and whitespace runs collapsed.
func CanonicalHeaders(r *http.Request, signed []string) string {
	var b strings.Builder
	for _, name := range signed {
		vals := headerValues(r, name)
		trimmed := make([]string, len(vals))
		for i, v := range vals {
			trimmed[i] = strings.Join(strings.Fields(v), " ")
		}
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(strings.Join(trimmed, ","))
		b.WriteByte('\n')
	}
	return b.String()
}

// CanonicalRequest assembles the canonical request string.
func CanonicalRequest(method, uri, query, headers string, signed []string, payloadHash string) string {
	return method + "\n" + uri + "\n" + query + "\n" + headers + "\n" +
		strings.Join(signed, ";") + "\n" + payloadHash
}

// Scope returns the credential scope for a date (YYYYMMDD) and region.
func Scope(date, region string) string {
	return date + "/" + region + "/" + Service + "/" + Terminator
}

// StringToSign returns the SigV4 string to sign for a canonical request.
func StringToSign(amzDate, scope, canonicalRequest string) string {
	return Algorithm + "\n" + amzDate + "\n" + scope + "\n" + hashHex([]byte(canonicalRequest))
}

// SigningKey derives the SigV4 signing key.
func SigningKey(secret, date, region string) []byte {
	k := hmacSHA256([]byte("AWS4"+secret), date)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, Service)
	return hmacSHA256(k, Terminator)
}

// Sign returns the hex signature of a string to sign.
func Sign(key []byte, stringToSign string) string {
	return hex.EncodeToString(hmacSHA256(key, stringToSign))
}

// ChunkSignature signs one aws-chunked chunk given the previous signature.
func ChunkSignature(key []byte, amzDate, scope, prevSig string, data []byte) string {
	return Sign(key, "AWS4-HMAC-SHA256-PAYLOAD\n"+amzDate+"\n"+scope+"\n"+prevSig+"\n"+
		EmptySHA256+"\n"+hashHex(data))
}

// TrailerSignature signs the trailer block; lines are "name:value\n" each.
func TrailerSignature(key []byte, amzDate, scope, prevSig, lines string) string {
	return Sign(key, "AWS4-HMAC-SHA256-TRAILER\n"+amzDate+"\n"+scope+"\n"+prevSig+"\n"+
		hashHex([]byte(lines)))
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func hashHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
