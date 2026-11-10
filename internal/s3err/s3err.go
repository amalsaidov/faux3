// Package s3err defines S3 error codes, their HTTP statuses, and the XML
// error document writer shared by the verifier and the API handlers.
package s3err

import (
	"encoding/xml"
	"errors"
	"net/http"
	"strings"
)

// Field is an extra element written into the XML error body, such as
// StringToSign on a SignatureDoesNotMatch error.
type Field struct {
	Name  string
	Value string
}

// Error is an S3 API error with its code, status, and message.
type Error struct {
	Code    string
	Status  int
	Message string
	Fields  []Field
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// With returns e with an extra XML element appended.
func (e *Error) With(name, value string) *Error {
	e.Fields = append(e.Fields, Field{Name: name, Value: value})
	return e
}

var statusByCode = map[string]int{
	"NoSuchBucket":                      http.StatusNotFound,
	"NoSuchKey":                         http.StatusNotFound,
	"BucketNotEmpty":                    http.StatusConflict,
	"BucketAlreadyOwnedByYou":           http.StatusConflict,
	"InvalidBucketName":                 http.StatusBadRequest,
	"InvalidLocationConstraint":         http.StatusBadRequest,
	"SignatureDoesNotMatch":             http.StatusForbidden,
	"InvalidAccessKeyId":                http.StatusForbidden,
	"AccessDenied":                      http.StatusForbidden,
	"RequestTimeTooSkewed":              http.StatusForbidden,
	"AuthorizationHeaderMalformed":      http.StatusBadRequest,
	"AuthorizationQueryParametersError": http.StatusBadRequest,
	"InvalidRequest":                    http.StatusBadRequest,
	"InvalidArgument":                   http.StatusBadRequest,
	"NotImplemented":                    http.StatusNotImplemented,
	"MethodNotAllowed":                  http.StatusMethodNotAllowed,
	"BadDigest":                         http.StatusBadRequest,
	"InvalidDigest":                     http.StatusBadRequest,
	"XAmzContentSHA256Mismatch":         http.StatusBadRequest,
	"IncompleteBody":                    http.StatusBadRequest,
	"MissingContentLength":              http.StatusLengthRequired,
	"InvalidRange":                      http.StatusRequestedRangeNotSatisfiable,
	"EntityTooLarge":                    http.StatusBadRequest,
	"KeyTooLongError":                   http.StatusBadRequest,
	"MetadataTooLarge":                  http.StatusBadRequest,
	"PreconditionFailed":                http.StatusPreconditionFailed,
	"InternalError":                     http.StatusInternalServerError,
}

// New returns an Error whose status comes from the S3 error table.
// Unknown codes map to 500.
func New(code, message string) *Error {
	status, ok := statusByCode[code]
	if !ok {
		status = http.StatusInternalServerError
	}
	return &Error{Code: code, Status: status, Message: message}
}

// As extracts an *Error from err, wrapping anything else as InternalError.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return New("InternalError", "We encountered an internal error. Please try again.")
}

// Write sends err as an S3 XML error document. HEAD responses carry the
// status only, with no body.
func Write(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	e := As(err)
	if r.Method == http.MethodHead {
		w.WriteHeader(e.Status)
		return
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<Error>")
	elem(&b, "Code", e.Code)
	elem(&b, "Message", e.Message)
	for _, f := range e.Fields {
		elem(&b, f.Name, f.Value)
	}
	elem(&b, "Resource", r.URL.EscapedPath())
	elem(&b, "RequestId", requestID)
	b.WriteString("</Error>")
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(e.Status)
	_, _ = w.Write([]byte(b.String()))
}

func elem(b *strings.Builder, name, value string) {
	b.WriteString("<" + name + ">")
	_ = xml.EscapeText(b, []byte(value))
	b.WriteString("</" + name + ">")
}
