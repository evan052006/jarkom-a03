// Package a03 defines the shared contract for the A03 student starter.
package a03

import (
	"errors"
	"time"
)

const (
	HTTPVersion           = "HTTP/1.1"
	MaxRequestHeaderBytes = 8192 // Includes the final CRLF CRLF.
	IOTimeout             = 5 * time.Second
)

// ErrNotImplemented identifies an exercise that has not been completed.
var ErrNotImplemented = errors.New("A03 exercise not implemented")

// Headers uses lowercase names (for example, "content-type"). Decoders must
// normalize incoming names; encoders must accept names in any capitalization.
// Duplicate headers and folded headers are outside the A03 grader's inputs.
type Headers map[string]string

// HttpRequest contains a GET request without a body. URI is the original
// origin-form target, including its optional query string.
type HttpRequest struct {
	Method  string
	URI     string
	Version string
	Headers Headers
}

// HttpResponse.Body holds decoded representation bytes. ResponseEncoder applies
// gzip when requested by Headers["content-encoding"] and calculates wire length.
// ResponseDecoder decompresses Body but preserves the wire Content-Length header.
type HttpResponse struct {
	Version    string
	StatusCode int
	Headers    Headers
	Body       []byte
}

type Student struct {
	Nama string
	Npm  string
}

// Default encoding/json and encoding/xml field names match the specification.
// encoding/xml uses GreetResponse as the root element.
type GreetResponse struct {
	Student Student
	Greeter string
}
