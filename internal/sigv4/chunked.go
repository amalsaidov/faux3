package sigv4

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/change-fabric/faux3/internal/s3err"
)

const (
	// MaxChunkSize caps one signed aws-chunked chunk so it can be
	// buffered and verified with bounded memory. Unsigned chunks stream
	// and have no cap: the AWS CLI sends a whole file as one chunk.
	MaxChunkSize = 16 << 20
	maxLineLen   = 4096
)

func incomplete(msg string) error {
	return s3err.New("IncompleteBody", "You did not provide the number of bytes specified by the Content-Length HTTP header; "+msg)
}

func badFrame(msg string) error {
	return s3err.New("InvalidArgument", "malformed aws-chunked body: "+msg)
}

func badChunkSig(what string) error {
	return s3err.New("SignatureDoesNotMatch", "The "+what+" signature we calculated does not match the signature you provided.")
}

// ChunkedReader decodes an aws-chunked body in any of the three supported
// streaming modes. Signed chunks are buffered (up to MaxChunkSize) and
// verified before any of their bytes are returned; unsigned chunks stream. Errors are
// *s3err.Error values; a successful read ends in io.EOF only after the
// final chunk, trailers, and decoded length all check out.
type ChunkedReader struct {
	br         *bufio.Reader
	v          *Verified
	signed     bool
	trailer    bool
	declared   map[string]bool
	decodedLen int64
	total      int64
	prevSig    string

	buf       []byte
	pos       int
	remaining int64 // bytes left in the current unsigned chunk

	done     bool
	err      error
	trailers map[string]string
}

// NewChunkedReader returns a decoder for body. trailerHeader is the
// x-amz-trailer request header (comma list) and decodedLength the
// x-amz-decoded-content-length value.
func NewChunkedReader(body io.Reader, v *Verified, trailerHeader string, decodedLength int64) (*ChunkedReader, error) {
	if !v.Mode.Streaming() {
		return nil, errors.New("sigv4: payload mode is not aws-chunked")
	}
	c := &ChunkedReader{
		br:         bufio.NewReaderSize(body, 64<<10),
		v:          v,
		signed:     v.Mode == PayloadStreamingSigned || v.Mode == PayloadStreamingSignedTrailer,
		trailer:    v.Mode == PayloadStreamingSignedTrailer || v.Mode == PayloadStreamingUnsignedTrailer,
		declared:   map[string]bool{},
		decodedLen: decodedLength,
		prevSig:    v.Signature,
		trailers:   map[string]string{},
	}
	for _, n := range strings.Split(trailerHeader, ",") {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			c.declared[n] = true
		}
	}
	if c.trailer && len(c.declared) == 0 {
		return nil, s3err.New("InvalidRequest", "x-amz-trailer is required for trailer payload modes")
	}
	return c, nil
}

// Trailers returns the trailing headers, keyed by lowercase name, once the
// reader has returned io.EOF.
func (c *ChunkedReader) Trailers() map[string]string { return c.trailers }

func (c *ChunkedReader) Read(p []byte) (int, error) {
	for {
		if c.pos < len(c.buf) {
			n := copy(p, c.buf[c.pos:])
			c.pos += n
			return n, nil
		}
		if c.err != nil {
			return 0, c.err
		}
		if c.remaining > 0 {
			return c.readUnsigned(p)
		}
		if c.done {
			return 0, io.EOF
		}
		c.err = c.nextChunk()
	}
}

func (c *ChunkedReader) readUnsigned(p []byte) (int, error) {
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.br.Read(p)
	c.remaining -= int64(n)
	switch {
	case err == io.EOF || err == io.ErrUnexpectedEOF:
		if c.remaining > 0 {
			c.err = incomplete("the chunked body ended early")
		}
	case err != nil:
		c.err = err
	}
	if c.remaining == 0 && c.err == nil {
		c.err = c.expectCRLF()
	}
	return n, nil
}

func (c *ChunkedReader) expectCRLF() error {
	var crlf [2]byte
	if _, err := io.ReadFull(c.br, crlf[:]); err != nil {
		return incomplete("the chunked body ended early")
	}
	if crlf != [2]byte{'\r', '\n'} {
		return badFrame("chunk data is not followed by CRLF")
	}
	return nil
}

// errCleanEOF marks EOF at the start of a line.
var errCleanEOF = errors.New("clean eof")

func (c *ChunkedReader) readLine() (string, error) {
	var line []byte
	for {
		frag, err := c.br.ReadSlice('\n')
		line = append(line, frag...)
		if len(line) > maxLineLen {
			return "", badFrame("line longer than 4096 bytes")
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			if len(line) == 0 && err == io.EOF {
				return "", errCleanEOF
			}
			if err == io.EOF {
				return "", incomplete("the chunked body ended early")
			}
			return "", err
		}
		break
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", badFrame("line not terminated by CRLF")
	}
	return string(line[:len(line)-2]), nil
}

func (c *ChunkedReader) nextChunk() error {
	line, err := c.readLine()
	if err == errCleanEOF {
		return incomplete("the chunked body ended before the final chunk")
	}
	if err != nil {
		return err
	}
	sizeStr, ext, _ := strings.Cut(line, ";")
	size, err := strconv.ParseInt(strings.TrimSpace(sizeStr), 16, 64)
	if err != nil || size < 0 {
		return badFrame("invalid chunk size " + quote(sizeStr))
	}
	if c.signed && size > MaxChunkSize {
		return s3err.New("InvalidArgument", "signed aws-chunked chunk size exceeds 16 MiB")
	}
	if c.total+size > c.decodedLen {
		return incomplete("the decoded body is longer than x-amz-decoded-content-length")
	}
	var sig string
	if c.signed {
		k, val, _ := strings.Cut(strings.TrimSpace(ext), "=")
		if k != "chunk-signature" || !isHex64(val) {
			return badChunkSig("chunk")
		}
		sig = strings.ToLower(val)
	}
	if size == 0 {
		return c.finish(sig)
	}
	c.total += size
	if !c.signed {
		c.remaining = size
		return nil
	}
	if int64(cap(c.buf)) < size {
		c.buf = make([]byte, size)
	}
	c.buf = c.buf[:size]
	c.pos = 0
	if _, err := io.ReadFull(c.br, c.buf); err != nil {
		c.buf = c.buf[:0]
		return incomplete("the chunked body ended early")
	}
	if err := c.verifyChunk(sig, c.buf); err != nil {
		c.buf = c.buf[:0]
		return err
	}
	if err := c.expectCRLF(); err != nil {
		c.buf = c.buf[:0]
		return err
	}
	return nil
}

func (c *ChunkedReader) verifyChunk(sig string, data []byte) error {
	want := ChunkSignature(c.v.SigningKey, c.v.AmzDate, c.v.Scope, c.prevSig, data)
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return badChunkSig("chunk")
	}
	c.prevSig = want
	return nil
}

func (c *ChunkedReader) finish(sig string) error {
	if c.signed {
		if err := c.verifyChunk(sig, nil); err != nil {
			return err
		}
	}
	if c.trailer {
		if err := c.readTrailers(); err != nil {
			return err
		}
	} else {
		line, err := c.readLine()
		if err != nil && err != errCleanEOF {
			return err
		}
		if line != "" {
			return badFrame("unexpected data after the final chunk")
		}
	}
	if c.total != c.decodedLen {
		return incomplete("the decoded body is shorter than x-amz-decoded-content-length")
	}
	c.done = true
	return nil
}

func (c *ChunkedReader) readTrailers() error {
	var lines strings.Builder
	var trailerSig string
	for {
		line, err := c.readLine()
		if err == errCleanEOF {
			break
		}
		if err != nil {
			return err
		}
		if line == "" {
			break
		}
		name, val, ok := strings.Cut(line, ":")
		if !ok {
			return badFrame("trailer line without a colon")
		}
		name = strings.ToLower(strings.TrimSpace(name))
		val = strings.TrimSpace(val)
		if name == "x-amz-trailer-signature" {
			trailerSig = strings.ToLower(val)
			continue
		}
		if !c.declared[name] {
			return s3err.New("InvalidRequest", "trailer "+quote(name)+" was not declared in x-amz-trailer")
		}
		c.trailers[name] = val
		lines.WriteString(name + ":" + val + "\n")
	}
	for name := range c.declared {
		if _, ok := c.trailers[name]; !ok {
			return incomplete("declared trailer " + quote(name) + " is missing")
		}
	}
	if c.signed {
		want := TrailerSignature(c.v.SigningKey, c.v.AmzDate, c.v.Scope, c.prevSig, lines.String())
		if !hmac.Equal([]byte(want), []byte(trailerSig)) {
			return badChunkSig("trailer")
		}
	}
	return nil
}

// sha256Reader hashes everything read through it.
type sha256Reader struct {
	r    io.Reader
	want string
	h    hashState
}

type hashState interface {
	io.Writer
	Sum([]byte) []byte
}

func (s *sha256Reader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	s.h.Write(p[:n])
	return n, err
}

func (s *sha256Reader) verify() error {
	if hex.EncodeToString(s.h.Sum(nil)) != s.want {
		return s3err.New("XAmzContentSHA256Mismatch", "The provided 'x-amz-content-sha256' header does not match what was computed.").
			With("ClientComputedContentSHA256", s.want).
			With("S3ComputedContentSHA256", hex.EncodeToString(s.h.Sum(nil)))
	}
	return nil
}

func newSHA256Reader(r io.Reader, want string) *sha256Reader {
	return &sha256Reader{r: r, want: want, h: sha256.New()}
}
