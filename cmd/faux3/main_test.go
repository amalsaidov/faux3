package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/change-fabric/faux3/internal/config"
	"github.com/change-fabric/faux3/internal/sigv4"
	"github.com/change-fabric/faux3/internal/store"
)

func TestProbeDataDirWritable(t *testing.T) {
	dir := t.TempDir()
	if err := probeDataDir(dir); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("probe left files behind: %v", entries)
	}
}

func TestProbeDataDirFailureNamesUID(t *testing.T) {
	err := probeDataDir(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"uid", "65532", "user:"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func testVerifier() *sigv4.Verifier {
	return &sigv4.Verifier{AccessKeyID: "faux3", SecretAccessKey: "faux3", Region: "us-east-1",
		ClockSkew: 15 * time.Minute, Now: func() time.Time { return testNow }}
}

func testServer(t *testing.T, log *slog.Logger) http.Handler {
	t.Helper()
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = t.TempDir()
	st, err := store.Open(store.Options{Dir: cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(log, cfg, st)
	srv.Verifier = testVerifier()
	return debugLog(log, srv)
}

func TestHandler(t *testing.T) {
	h := testServer(t, slog.New(slog.NewTextHandler(io.Discard, nil)))
	signer := sigv4.Signer{AccessKeyID: "faux3", SecretAccessKey: "faux3", Region: "us-east-1"}
	cases := []struct {
		name string
		req  func() *http.Request
		code int
		body string
	}{
		{"healthz", func() *http.Request { return httptest.NewRequest(http.MethodGet, "/-/healthz", nil) }, 200, "ok"},
		{"unauthenticated", func() *http.Request { return httptest.NewRequest(http.MethodGet, "/bucket", nil) }, 403, "<Code>AccessDenied</Code>"},
		{"signed", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/bucket?list-type=2", nil)
			signer.SignHeader(r, testNow, sigv4.EmptySHA256)
			return r
		}, 404, "<Code>NoSuchBucket</Code>"},
		{"presigned", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/bucket/key", nil)
			signer.Presign(r, testNow, time.Hour)
			return r
		}, 404, "<Code>NoSuchBucket</Code>"},
		{"wrong secret", func() *http.Request {
			s := signer
			s.SecretAccessKey = "nope"
			r := httptest.NewRequest(http.MethodGet, "/bucket", nil)
			s.SignHeader(r, testNow, sigv4.EmptySHA256)
			return r
		}, 403, "<StringToSign>"},
		{"head error has no body", func() *http.Request { return httptest.NewRequest(http.MethodHead, "/bucket", nil) }, 403, ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, c.req())
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.body) || (c.body == "" && rec.Body.Len() != 0) {
			t.Errorf("%s: %d %q", c.name, rec.Code, rec.Body)
		}
		if c.name != "healthz" && rec.Header().Get("x-amz-request-id") == "" {
			t.Errorf("%s: missing x-amz-request-id", c.name)
		}
	}
}

func TestVersionAndUnknown(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := run([]string{"version"}, &out, &errb); rc != 0 || strings.TrimSpace(out.String()) != version {
		t.Fatalf("version: rc=%d out=%q", rc, out.String())
	}
	if rc := run([]string{"bogus"}, &out, &errb); rc != 2 {
		t.Fatalf("unknown: rc=%d", rc)
	}
}

func TestDebugLogRedactsAndLogsWireHeaders(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	req := httptest.NewRequest(http.MethodPut, "/b/k", strings.NewReader("hello"))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=x, SignedHeaders=host, Signature=0123456789abcdef")
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	req.Header.Set("X-Amz-Checksum-Crc32", "AAAAAA==")
	testServer(t, log).ServeHTTP(httptest.NewRecorder(), req)
	out := buf.String()
	for _, want := range []string{"x-amz-content-sha256=UNSIGNED-PAYLOAD", "x-amz-checksum-crc32", "Signature=01234567..."} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "0123456789abcdef") {
		t.Fatal("full signature logged")
	}
}
