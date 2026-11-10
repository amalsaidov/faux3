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

func TestHandler(t *testing.T) {
	h := newHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/-/healthz", nil))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("healthz: %d %q", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bucket", nil))
	if rec.Code != 501 || !strings.Contains(rec.Body.String(), "<Code>NotImplemented</Code>") {
		t.Fatalf("other: %d %q", rec.Code, rec.Body)
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
	newHandler(log).ServeHTTP(httptest.NewRecorder(), req)
	out := buf.String()
	for _, want := range []string{"x-amz-content-sha256=UNSIGNED-PAYLOAD", "x-amz-checksum-crc32", "Signature=01234567...", "bytes=5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "0123456789abcdef") {
		t.Fatal("full signature logged")
	}
}
