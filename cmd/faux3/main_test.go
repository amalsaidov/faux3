package main

import (
	"bytes"
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
	h := newHandler()
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
