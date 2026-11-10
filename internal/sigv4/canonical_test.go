package sigv4

import (
	"encoding/hex"
	"net/http"
	"testing"
)

func TestURIEncode(t *testing.T) {
	cases := []struct {
		in    string
		slash bool
		want  string
	}{
		{"abcAZ09-._~", true, "abcAZ09-._~"},
		{"a b+c", true, "a%20b%2Bc"},
		{"a/b", false, "a/b"},
		{"a/b", true, "a%2Fb"},
		{"é", true, "%C3%A9"},
		{"*", true, "%2A"},
	}
	for _, c := range cases {
		if got := URIEncode(c.in, c.slash); got != c.want {
			t.Errorf("URIEncode(%q,%v)=%q want %q", c.in, c.slash, got, c.want)
		}
	}
}

func TestCanonicalURI(t *testing.T) {
	cases := map[string]string{
		"":                       "/",
		"/":                      "/",
		"/test%24file.text":      "/test%24file.text",
		"/b/a%20b":               "/b/a%20b",
		"/b/a%2bb":               "/b/a%2Bb",
		"/b/a+b":                 "/b/a%2Bb",
		"/b//x/../y":             "/b//x/../y",
		"/b/%E2%9C%93":           "/b/%E2%9C%93",
		"/b/a%2Fb":               "/b/a/b",
		"/b/%25":                 "/b/%25",
		"/examplebucket/my(key)": "/examplebucket/my%28key%29",
	}
	for in, want := range cases {
		if got := CanonicalURI(in); got != want {
			t.Errorf("CanonicalURI(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCanonicalQuery(t *testing.T) {
	cases := []struct{ raw, skip, want string }{
		{"", "", ""},
		{"lifecycle", "", "lifecycle="},
		{"prefix=J&max-keys=2", "", "max-keys=2&prefix=J"},
		{"prefix=a%2Fb&list-type=2", "", "list-type=2&prefix=a%2Fb"},
		{"prefix=a/b", "", "prefix=a%2Fb"},
		{"prefix=a%20b", "", "prefix=a%20b"},
		{"b=2&a=2&a=1", "", "a=1&a=2&b=2"},
		{"X-Amz-Signature=abc&x=1", "X-Amz-Signature", "x=1"},
		{"&&a=1&", "", "a=1"},
	}
	for _, c := range cases {
		if got := CanonicalQuery(c.raw, c.skip); got != c.want {
			t.Errorf("CanonicalQuery(%q)=%q want %q", c.raw, got, c.want)
		}
	}
}

func TestCanonicalHeaders(t *testing.T) {
	r := newReq(http.MethodGet, "/b", nil)
	r.Header.Add("X-Amz-Meta-A", "  one   two ")
	r.Header.Add("X-Amz-Meta-A", "three")
	got := CanonicalHeaders(r, []string{"host", "x-amz-meta-a"})
	want := "host:localhost:9000\nx-amz-meta-a:one two,three\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// From the AWS SigV4 docs (s3 GET object example).
func TestDocGetObjectCanonicalRequest(t *testing.T) {
	r := newReq(http.MethodGet, "http://examplebucket.s3.amazonaws.com/test.txt", nil)
	r.Host = "examplebucket.s3.amazonaws.com"
	r.Header.Set("Range", "bytes=0-9")
	r.Header.Set("X-Amz-Content-Sha256", EmptySHA256)
	r.Header.Set("X-Amz-Date", "20130524T000000Z")
	signed := []string{"host", "range", "x-amz-content-sha256", "x-amz-date"}
	creq := CanonicalRequest("GET", CanonicalURI(r.URL.EscapedPath()), CanonicalQuery(r.URL.RawQuery, ""),
		CanonicalHeaders(r, signed), signed, EmptySHA256)
	want := "GET\n/test.txt\n\nhost:examplebucket.s3.amazonaws.com\nrange:bytes=0-9\n" +
		"x-amz-content-sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n" +
		"x-amz-date:20130524T000000Z\n\nhost;range;x-amz-content-sha256;x-amz-date\n" +
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if creq != want {
		t.Fatalf("canonical request\n%s\nwant\n%s", creq, want)
	}
	sts := StringToSign("20130524T000000Z", Scope("20130524", "us-east-1"), creq)
	wantSTS := "AWS4-HMAC-SHA256\n20130524T000000Z\n20130524/us-east-1/s3/aws4_request\n" +
		"7344ae5b7ee6c3e7e6b0fe0640412a37625d1fbfff95c48bbb2dc43964946972"
	if sts != wantSTS {
		t.Fatalf("string to sign\n%s\nwant\n%s", sts, wantSTS)
	}
	sig := Sign(SigningKey(docSecret, "20130524", "us-east-1"), sts)
	if sig != "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41" {
		t.Fatalf("signature %s", sig)
	}
}

// Signing key example from the AWS SigV4 docs (iam, us-east-1, 20150830).
func TestSigningKeyDocVector(t *testing.T) {
	k := hmacSHA256([]byte("AWS4wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"), "20150830")
	k = hmacSHA256(k, "us-east-1")
	k = hmacSHA256(k, "iam")
	k = hmacSHA256(k, "aws4_request")
	if got := hex.EncodeToString(k); got != "c4afb1cc5771d871763a393e44b703571b55cc28424d1a5e86da6ed3c154a4b9" {
		t.Fatalf("signing key %s", got)
	}
}
