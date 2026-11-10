package sigv4

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// The chunked upload example from the AWS S3 SigV4 streaming docs:
// PUT examplebucket/chunkObject.txt, 66560 bytes of 'a' in 64 KiB chunks.
func TestDocChunkedExample(t *testing.T) {
	sigs := []string{
		"ad80c730a21e5b8d04586a2213dd63b9a0e99e0e2307b0ade35a65485a288648",
		"0055627c9e194cb4542bae2aa5492e3c1575bbb81b612b7d234b86a503ef5497",
		"b6c6ea8a5354eaf15b3cb7646744f4275b71ea724fed81ceb9323e279d449df9",
	}
	build := func(sigs []string, body1 []byte) []byte {
		var b bytes.Buffer
		b.WriteString("10000;chunk-signature=" + sigs[0] + "\r\n")
		b.Write(body1)
		b.WriteString("\r\n400;chunk-signature=" + sigs[1] + "\r\n")
		b.Write(bytes.Repeat([]byte("a"), 1024))
		b.WriteString("\r\n0;chunk-signature=" + sigs[2] + "\r\n\r\n")
		return b.Bytes()
	}
	req := func(body []byte) *http.Request {
		r := newReq("PUT", "/examplebucket/chunkObject.txt", body)
		r.Host = "s3.amazonaws.com"
		r.Header.Set("X-Amz-Date", "20130524T000000Z")
		r.Header.Set("X-Amz-Storage-Class", "REDUCED_REDUNDANCY")
		r.Header.Set("Content-Encoding", "aws-chunked")
		r.Header.Set("X-Amz-Decoded-Content-Length", "66560")
		r.Header.Set("Content-Length", "66824")
		r.Header.Set("X-Amz-Content-Sha256", StreamingSigned)
		r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+docAK+"/20130524/us-east-1/s3/aws4_request,"+
			"SignedHeaders=content-encoding;content-length;host;x-amz-content-sha256;x-amz-date;x-amz-decoded-content-length;x-amz-storage-class,"+
			"Signature=4f232c4386841ef735655705268965c44a0e4690baa4adea153f7db9fa80a0a9")
		return r
	}
	good := build(sigs, bytes.Repeat([]byte("a"), 65536))
	if len(good) != 66824 {
		t.Fatalf("framed length %d", len(good))
	}
	read := func(body []byte) ([]byte, error) {
		r := req(body)
		vf, err := docVerifier().Verify(r)
		if err != nil {
			return nil, err
		}
		p, err := vf.Payload(r)
		if err != nil {
			return nil, err
		}
		return io.ReadAll(p)
	}
	out, err := read(good)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, bytes.Repeat([]byte("a"), 66560)) {
		t.Fatalf("decoded %d bytes", len(out))
	}

	tampered := bytes.Repeat([]byte("a"), 65536)
	tampered[100] = 'b'
	out, err = read(build(sigs, tampered))
	wantCode(t, err, "SignatureDoesNotMatch")
	if len(out) != 0 {
		t.Fatalf("returned %d bytes of an unverified chunk", len(out))
	}

	badFinal := append([]string{}, sigs...)
	badFinal[2] = strings.Repeat("0", 64)
	out, err = read(build(badFinal, bytes.Repeat([]byte("a"), 65536)))
	wantCode(t, err, "SignatureDoesNotMatch")
	if len(out) != 66560 {
		t.Fatalf("verified chunks should be returned, got %d", len(out))
	}
}

type chunkCase struct {
	mode     string
	signed   bool
	trailer  bool
	signTr   bool
	declared string
}

var chunkModes = []chunkCase{
	{mode: StreamingSigned, signed: true},
	{mode: StreamingUnsignedTrailer, trailer: true, declared: "x-amz-checksum-crc32"},
	{mode: StreamingSignedTrailer, signed: true, trailer: true, signTr: true, declared: "x-amz-checksum-crc32c"},
}

// streamReq builds a signed aws-chunked request with the reference signer.
func streamReq(t *testing.T, c chunkCase, data []byte, chunkSize int, framed func([]byte) []byte, chunkedTE bool, pre ...func(*http.Request)) *http.Request {
	t.Helper()
	r := newReq("PUT", "/b/k", nil)
	r.Header.Set("Content-Encoding", "aws-chunked")
	r.Header.Set("X-Amz-Decoded-Content-Length", strconv.Itoa(len(data)))
	if c.declared != "" {
		r.Header.Set("X-Amz-Trailer", c.declared)
	}
	for _, f := range pre {
		f(r)
	}
	seed := testSigner.SignHeader(r, t0, c.mode)
	spec := chunkSpec{key: SigningKey("faux3", "20260101", "us-east-1"), amzDate: "20260101T120000Z",
		scope: Scope("20260101", "us-east-1"), seed: seed, signed: c.signed, chunkSize: chunkSize, signTrail: c.signTr}
	if c.trailer {
		spec.trailers = [][2]string{{c.declared, "AAAAAA=="}}
	}
	body := encodeChunked(spec, data)
	if framed != nil {
		body = framed(body)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if chunkedTE {
		r.ContentLength = -1
		r.TransferEncoding = []string{"chunked"}
	} else {
		r.ContentLength = int64(len(body))
	}
	return r
}

func decode(r *http.Request) ([]byte, *Payload, error) {
	vf, err := testVerifier().Verify(r)
	if err != nil {
		return nil, nil, err
	}
	p, err := vf.Payload(r)
	if err != nil {
		return nil, nil, err
	}
	b, err := io.ReadAll(p)
	return b, p, err
}

func TestChunkedRoundTrip(t *testing.T) {
	data := make([]byte, 200_000)
	for i := range data {
		data[i] = byte(i * 7)
	}
	for _, c := range chunkModes {
		for _, size := range []int{1, 8192, 65536, 200_000} {
			for _, te := range []bool{false, true} {
				name := fmt.Sprintf("%s/%d/te=%v", c.mode, size, te)
				t.Run(name, func(t *testing.T) {
					d := data
					if size == 1 {
						d = data[:300]
					}
					got, p, err := decode(streamReq(t, c, d, size, nil, te))
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(got, d) {
						t.Fatalf("decoded mismatch: %d vs %d bytes", len(got), len(d))
					}
					if c.trailer && p.Trailers()[c.declared] != "AAAAAA==" {
						t.Fatalf("trailers %v", p.Trailers())
					}
				})
			}
		}
	}
}

func TestChunkedEmptyBody(t *testing.T) {
	for _, c := range chunkModes {
		got, _, err := decode(streamReq(t, c, nil, 65536, nil, false))
		if err != nil || len(got) != 0 {
			t.Fatalf("%s: %v %d", c.mode, err, len(got))
		}
	}
}

// The exact unsigned-trailer framing captured from the JS SDK on the wire.
func TestChunkedWireCapture(t *testing.T) {
	body := "19\r\nhello faux3 wire capture\n\r\n0\r\nx-amz-checksum-crc32:X9wasw==\r\n\r\n"
	r := newReq("PUT", "/b/k", nil)
	r.Header.Set("Content-Encoding", "aws-chunked")
	r.Header.Set("X-Amz-Decoded-Content-Length", "25")
	r.Header.Set("X-Amz-Trailer", "x-amz-checksum-crc32")
	r.Header.Set("X-Amz-Sdk-Checksum-Algorithm", "CRC32")
	testSigner.SignHeader(r, t0, StreamingUnsignedTrailer)
	r.Body = io.NopCloser(strings.NewReader(body))
	r.ContentLength = -1
	got, p, err := decode(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello faux3 wire capture\n" || p.Trailers()["x-amz-checksum-crc32"] != "X9wasw==" {
		t.Fatalf("got %q %v", got, p.Trailers())
	}
}

func TestChunkedNegative(t *testing.T) {
	data := bytes.Repeat([]byte("xyz"), 30000)
	cut := func(n int) func([]byte) []byte { return func(b []byte) []byte { return b[:len(b)-n] } }
	flip := func(off int) func([]byte) []byte {
		return func(b []byte) []byte {
			b = append([]byte{}, b...)
			i := bytes.Index(b, []byte("\r\n")) + 2 + off
			b[i] ^= 1
			return b
		}
	}
	type neg struct {
		name   string
		c      chunkCase
		framed func([]byte) []byte
		mut    func(*http.Request)
		code   string
	}
	var cases []neg
	for _, c := range chunkModes {
		cases = append(cases,
			neg{"truncated mid data", c, func(b []byte) []byte { return b[:1000] }, nil, "IncompleteBody"},
			neg{"truncated before final", c, func(b []byte) []byte { return b[:bytes.LastIndex(b, []byte("\r\n0"))+2] }, nil, "IncompleteBody"},
			neg{"decoded length too large", c, nil, func(r *http.Request) { r.Header.Set("X-Amz-Decoded-Content-Length", "90001") }, "IncompleteBody"},
			neg{"decoded length too small", c, nil, func(r *http.Request) { r.Header.Set("X-Amz-Decoded-Content-Length", "10") }, "IncompleteBody"},
		)
		if c.signed {
			cases = append(cases, neg{"tampered data", c, flip(10), nil, "SignatureDoesNotMatch"})
		}
		if c.trailer {
			cases = append(cases,
				neg{"truncated in trailer", c, cut(6), nil, "IncompleteBody"},
				neg{"undeclared trailer", c, func(b []byte) []byte {
					return bytes.Replace(b, []byte(c.declared+":"), []byte("x-amz-checksum-sha1:"), 1)
				}, nil, "InvalidRequest"},
			)
		}
		if c.signTr {
			cases = append(cases, neg{"tampered trailer", c, func(b []byte) []byte {
				return bytes.Replace(b, []byte("AAAAAA=="), []byte("AAAAAB=="), 1)
			}, nil, "SignatureDoesNotMatch"})
		}
	}
	cases = append(cases,
		neg{"bad chunk size", chunkModes[1], func(b []byte) []byte { return append([]byte("zz"), b[bytes.IndexByte(b, '\r'):]...) }, nil, "InvalidArgument"},
		neg{"long header line", chunkModes[1], func(b []byte) []byte { return append(bytes.Repeat([]byte("0"), 5000), b...) }, nil, "InvalidArgument"},
		neg{"oversize chunk", chunkModes[1], func([]byte) []byte { return []byte("1000001\r\n") }, func(r *http.Request) {
			r.Header.Set("X-Amz-Decoded-Content-Length", "99999999")
		}, "InvalidArgument"},
		neg{"missing chunk signature", chunkModes[0], func(b []byte) []byte {
			i := bytes.Index(b, []byte(";chunk-signature="))
			return append(append([]byte{}, b[:i]...), b[i+17+64:]...)
		}, nil, "SignatureDoesNotMatch"},
	)
	for _, n := range cases {
		t.Run(n.c.mode+"/"+n.name, func(t *testing.T) {
			var pre []func(*http.Request)
			if n.mut != nil {
				pre = append(pre, n.mut)
			}
			r := streamReq(t, n.c, data, 65536, n.framed, false, pre...)
			_, _, err := decode(r)
			wantCode(t, err, n.code)
		})
	}
}

func TestMissingDecodedLength(t *testing.T) {
	r := streamReq(t, chunkModes[1], []byte("abc"), 10, nil, false, func(r *http.Request) {
		r.Header.Del("X-Amz-Decoded-Content-Length")
	})
	vf, err := testVerifier().Verify(r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = vf.Payload(r)
	wantCode(t, err, "MissingContentLength")
}

func TestStripAWSChunked(t *testing.T) {
	cases := map[string]string{"aws-chunked": "", "aws-chunked,gzip": "gzip", "gzip, aws-chunked": "gzip", "": ""}
	for in, want := range cases {
		if got := StripAWSChunked(in); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
}
