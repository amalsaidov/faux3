package store

import (
	"errors"
	"reflect"
	"testing"
)

func listKeys(r ListResult) []string {
	out := []string{}
	for _, m := range r.Objects {
		out = append(out, m.Key)
	}
	return out
}

func TestListByteOrder(t *testing.T) {
	s, _ := newBucket(t)
	for _, k := range []string{"b", "a0", "a/", "a", "B", "\u00e9", "Z"} {
		put(t, s, k, "x")
	}
	r, err := s.List("bkt", ListParams{MaxKeys: 1000})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"B", "Z", "a", "a/", "a0", "b", "\u00e9"}
	if got := listKeys(r); !reflect.DeepEqual(got, want) || r.IsTruncated {
		t.Fatalf("got %q truncated=%v", got, r.IsTruncated)
	}
}

func TestListTable(t *testing.T) {
	s, _ := newBucket(t)
	for _, k := range []string{"a/1", "a/2", "a/b/3", "b/1", "c", "d/1", "d/2", "e"} {
		put(t, s, k, "x")
	}
	cases := []struct {
		name   string
		p      ListParams
		keys   []string
		cps    []string
		trunc  bool
		resume string
	}{
		{"all", ListParams{MaxKeys: 1000}, []string{"a/1", "a/2", "a/b/3", "b/1", "c", "d/1", "d/2", "e"}, nil, false, ""},
		{"prefix", ListParams{Prefix: "a/", MaxKeys: 1000}, []string{"a/1", "a/2", "a/b/3"}, nil, false, ""},
		{"delimiter", ListParams{Delimiter: "/", MaxKeys: 1000}, []string{"c", "e"}, []string{"a/", "b/", "d/"}, false, ""},
		{"prefix+delimiter", ListParams{Prefix: "a/", Delimiter: "/", MaxKeys: 1000}, []string{"a/1", "a/2"}, []string{"a/b/"}, false, ""},
		{"max2 rollup", ListParams{Delimiter: "/", MaxKeys: 2}, []string{}, []string{"a/", "b/"}, true, "b/"},
		{"start-after", ListParams{StartAfter: "b/1", MaxKeys: 1000}, []string{"c", "d/1", "d/2", "e"}, nil, false, ""},
		{"zero", ListParams{MaxKeys: 0}, []string{}, nil, false, ""},
		{"no match", ListParams{Prefix: "zz", MaxKeys: 1000}, []string{}, nil, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := s.List("bkt", c.p)
			if err != nil {
				t.Fatal(err)
			}
			if got := listKeys(r); !reflect.DeepEqual(got, c.keys) {
				t.Fatalf("keys %q want %q", got, c.keys)
			}
			if !reflect.DeepEqual(r.CommonPrefixes, c.cps) {
				t.Fatalf("cps %q want %q", r.CommonPrefixes, c.cps)
			}
			if r.IsTruncated != c.trunc {
				t.Fatalf("truncated %v", r.IsTruncated)
			}
			if c.trunc && r.NextContinuationToken != EncodeToken(c.resume) {
				t.Fatalf("token %q", r.NextContinuationToken)
			}
		})
	}
}

// TestListPaginate walks every page size and checks the concatenation
// equals the unpaginated listing, with common prefixes never repeated.
func TestListPaginate(t *testing.T) {
	s, _ := newBucket(t)
	for _, k := range []string{"a/1", "a/2", "a/b/3", "b/1", "c", "d/1", "d/2", "e"} {
		put(t, s, k, "x")
	}
	for _, delim := range []string{"", "/"} {
		full, _ := s.List("bkt", ListParams{Delimiter: delim, MaxKeys: 1000})
		for size := 1; size <= 4; size++ {
			var keys, cps []string
			tok := ""
			for pages := 0; ; pages++ {
				if pages > 20 {
					t.Fatal("pagination loops")
				}
				r, err := s.List("bkt", ListParams{Delimiter: delim, MaxKeys: size, ContinuationToken: tok})
				if err != nil {
					t.Fatal(err)
				}
				keys = append(keys, listKeys(r)...)
				cps = append(cps, r.CommonPrefixes...)
				if len(r.Objects)+len(r.CommonPrefixes) > size {
					t.Fatalf("page over max-keys")
				}
				if !r.IsTruncated {
					break
				}
				tok = r.NextContinuationToken
			}
			if !reflect.DeepEqual(keys, listKeys(full)) || !reflect.DeepEqual(cps, full.CommonPrefixes) {
				t.Fatalf("delim %q size %d: keys %q cps %q", delim, size, keys, cps)
			}
		}
	}
}

func TestListErrors(t *testing.T) {
	s, _ := newBucket(t)
	if _, err := s.List("bkt", ListParams{MaxKeys: -1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("negative = %v", err)
	}
	if _, err := s.List("bkt", ListParams{MaxKeys: 10, ContinuationToken: "!!!"}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("token = %v", err)
	}
	if _, err := s.List("nope", ListParams{MaxKeys: 10}); !errors.Is(err, ErrNoSuchBucket) {
		t.Fatalf("bucket = %v", err)
	}
	for i := 0; i < 1001; i++ {
		put(t, s, string(rune('a'+i%26))+string(rune('0'+i/26%10))+string(rune('A'+i/260)), "")
	}
	r, _ := s.List("bkt", ListParams{MaxKeys: 5000})
	if len(r.Objects) != 1000 || !r.IsTruncated {
		t.Fatalf("cap: %d %v", len(r.Objects), r.IsTruncated)
	}
}

func TestSuccessor(t *testing.T) {
	if s, ok := successor("a/"); !ok || s != "a0" {
		t.Fatalf("%q %v", s, ok)
	}
	if s, ok := successor("a\xff"); !ok || s != "b" {
		t.Fatalf("%q %v", s, ok)
	}
	if _, ok := successor("\xff\xff"); ok {
		t.Fatal("want none")
	}
}
