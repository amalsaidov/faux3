package store

import (
	"net/netip"
	"strings"
	"unicode/utf8"
)

// ValidBucketName reports whether name follows the S3 bucket naming rules.
func ValidBucketName(name string) bool {
	if len(name) < 3 || len(name) > 63 {
		return false
	}
	isAlnum := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' }
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(isAlnum(c) || c == '.' || c == '-') {
			return false
		}
	}
	if !isAlnum(name[0]) || !isAlnum(name[len(name)-1]) {
		return false
	}
	if strings.Contains(name, "..") || strings.HasPrefix(name, "xn--") || strings.HasSuffix(name, "-s3alias") {
		return false
	}
	if a, err := netip.ParseAddr(name); err == nil && a.Is4() {
		return false
	}
	return true
}

// ValidateKey checks an object key: valid UTF-8, 1..1024 bytes.
func ValidateKey(key string) error {
	if len(key) > maxKeyBytes {
		return ErrKeyTooLong
	}
	if key == "" || !utf8.ValidString(key) {
		return ErrInvalidKey
	}
	return nil
}
