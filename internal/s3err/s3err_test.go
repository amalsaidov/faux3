package s3err

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewStatus(t *testing.T) {
	cases := map[string]int{
		"SignatureDoesNotMatch": 403,
		"IncompleteBody":        400,
		"MissingContentLength":  411,
		"NotImplemented":        501,
		"Unknown":               500,
	}
	for code, want := range cases {
		if got := New(code, "m").Status; got != want {
			t.Errorf("%s: status %d, want %d", code, got, want)
		}
	}
}

func TestWriteXML(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/b/k", nil)
	Write(w, r, New("SignatureDoesNotMatch", "bad <sig>").With("StringToSign", "a\nb"), "REQ1")
	if w.Code != 403 {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"<Code>SignatureDoesNotMatch</Code>", "bad &lt;sig&gt;", "<StringToSign>a&#xA;b</StringToSign>", "<RequestId>REQ1</RequestId>", "<Resource>/b/k</Resource>"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
}

func TestWriteHeadNoBody(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodHead, "/b", nil)
	Write(w, r, New("AccessDenied", "x"), "R")
	if w.Code != 403 || w.Body.Len() != 0 {
		t.Fatalf("status %d body %q", w.Code, w.Body.String())
	}
}

func TestAsWrapsUnknown(t *testing.T) {
	if As(errors.New("boom")).Code != "InternalError" {
		t.Fatal("expected InternalError")
	}
}
