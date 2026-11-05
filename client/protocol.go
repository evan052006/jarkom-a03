package main

import (
	"a03"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Grader contract: retain these aliases and all exported function signatures.
type HttpRequest = a03.HttpRequest
type HttpResponse = a03.HttpResponse

func ContainAnyRune(s string, runs string) bool {
	for _, v := range s {
		if strings.ContainsRune(runs, v) {
			return true
		}
	}
	return false
}

func ContainAllRune(s string, runs string) bool {
	for _, v := range s {
		if !strings.ContainsRune(runs, v) {
			return false
		}
	}
	return true
}

// RequestEncoder builds the request line, headers, and terminating empty line.
// Encode the supplied headers and always send Connection: close, replacing any
// supplied value. Omit Accept-Encoding unless the caller set it (gzip). Return an
// error, never panic, for a missing/empty Host or Accept, CR or LF in a name or
// value, a colon or whitespace in a header name, whitespace in the URI, an empty
// or non-token Method, or a Version other than empty or HTTP/1.1. Always write
// HTTP/1.1 on the request line.
func RequestEncoder(request HttpRequest) ([]byte, error) {
	// TODO A: encode a request with CRLF line endings, without net/http.

	hostExist := false
	acceptExist := false
	for k, v := range request.Headers {
		if strings.EqualFold(k, "host") && v != "" {
			hostExist = true
		}
		if strings.EqualFold(k, "accept") && v != "" {
			acceptExist = true
		}
		if ContainAnyRune(k, "\r\n") || ContainAnyRune(v, "\r\n") {
			return nil, fmt.Errorf("CR or LF in header name or value")
		}
		if ContainAnyRune(k, ": \t") {
			return nil, fmt.Errorf("Colon or white space in header name")
		}
	}

	if !hostExist {
		return nil, fmt.Errorf("Host header does not exist")
	}
	if !acceptExist {
		return nil, fmt.Errorf("Accept header does not exist")
	}

	if len(request.Method) == 0 {
		return nil, fmt.Errorf("Method is empty")
	}

	if !ContainAllRune(request.Method, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&'*+-.^_`|~") {
		return nil, fmt.Errorf("Method does not adhere to RFC 9110 token")
	}

	if len(request.Version) != 0 && strings.Compare(request.Version, "HTTP/1.1") != 0 {
		return nil, fmt.Errorf("Version is not empty or exactly HTTP/1.1")
	}

	if strings.ContainsRune(request.URI, ' ') {
		return nil, fmt.Errorf("whitespace in the URI")
	}

	result := []byte(request.Method)
	result = append(result, ' ')
	result = append(result, []byte(request.URI)...)
	result = append(result, ' ')
	result = append(result, []byte("HTTP/1.1")...)
	result = append(result, []byte("\r\n")...)

	for k, v := range request.Headers {
		if strings.EqualFold(k, "Connection") {
			continue
		}
		if strings.EqualFold(k, "Accept-Encoding") && v != "gzip" {
			continue
		}
		result = append(result, []byte(k)...)
		result = append(result, []byte(": ")...)
		result = append(result, []byte(v)...)
		result = append(result, []byte("\r\n")...)
	}

	result = append(result, []byte("Connection: close\r\n")...)

	result = append(result, []byte("\r\n")...)

	return result, nil
}

// ResponseDecoder validates a complete HTTP/1.1 response and its Content-Length.
// Body must contain exactly the declared number of wire bytes before optional
// gzip decompression. Keep Content-Length unchanged in the decoded headers.
func ResponseDecoder(message []byte) (HttpResponse, error) {
	// TODO B: parse the status line and case-insensitive headers, validate framing,
	// and decompress gzip. Reject truncated headers/bodies and invalid lengths.
	headerEndIdx := bytes.Index(message, []byte("\r\n\r\n"))
	if headerEndIdx == -1 {
		return HttpResponse{}, fmt.Errorf("Message doesn't contain CRLF CRLF")
	}

	lines_str := string(message[:headerEndIdx])

	lines := strings.Split(lines_str, "\r\n")

	statusLine := strings.SplitN(lines[0], " ", 3)
	if len(statusLine) != 3 && len(statusLine) != 2 {
		return HttpResponse{}, fmt.Errorf("Status line malformed")
	}

	version := statusLine[0]
	statusCodeString := statusLine[1]
	// reasonPhrase := statusLine[2]

	if len(version) != 0 && strings.Compare(version, "HTTP/1.1") != 0 {
		return HttpResponse{}, fmt.Errorf("Version is not empty or exactly HTTP/1.1")
	}

	statusCode, err := strconv.Atoi(statusCodeString)
	if err != nil {
		return HttpResponse{}, fmt.Errorf("Status code is not integer")
	}

	headers_str := lines[1:]

	contentLength := -1

	useGzip := false

	header_map := map[string]string{}
	for _, line := range headers_str {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			valStr := strings.TrimSpace(parts[1])
			keyStr := strings.ToLower(parts[0])
			header_map[keyStr] = valStr
			if keyStr == "content-length" {
				contentLength, err = strconv.Atoi(valStr)
				if err != nil {
					return HttpResponse{}, fmt.Errorf("Failed parsing Content-Length: %s", err)
				}
			}
			if keyStr == "content-encoding" && valStr == "gzip" {
				useGzip = true
			}
		} else {
			return HttpResponse{}, fmt.Errorf("Header format malformed")
		}
	}

	if contentLength == -1 {
		return HttpResponse{}, fmt.Errorf("Content-Length not found in header")
	}

	body := message[headerEndIdx+4:]

	if len(body) != contentLength {
		return HttpResponse{}, fmt.Errorf("Body size does not follow content-length")
	}

	if useGzip {
		gzipReader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return HttpResponse{}, fmt.Errorf("Failed to create gzip reader")
		}
		defer gzipReader.Close()
		decompressedBody, err := io.ReadAll(gzipReader)
		if err != nil {
			return HttpResponse{}, fmt.Errorf("Failed to decompress body: %w", err)
		}
		body = decompressedBody
	}

	return HttpResponse{Version: version, StatusCode: statusCode, Headers: header_map, Body: body}, nil
}

// Fetch sends one request and reads one complete response. It must use
// RequestEncoder and ResponseDecoder. The caller owns and closes conn.
func Fetch(request HttpRequest, conn net.Conn) (HttpResponse, error) {
	// TODO C: set a finite deadline, write all request bytes, read through CRLF CRLF,
	// then read exactly Content-Length body bytes. A Read is not a message boundary.
	// A buffered reader may already contain body bytes: do not discard them.

	if err := conn.SetDeadline(time.Now().Add(a03.IOTimeout)); err != nil {
		return HttpResponse{}, fmt.Errorf("deadline: %w", err)
	}

	request_bytes, err := RequestEncoder(request)
	if err != nil {
		return HttpResponse{}, fmt.Errorf("Failed parsing request %s", err)
	}

	_, err = conn.Write(request_bytes)
	if err != nil {
		return HttpResponse{}, fmt.Errorf("Failed sending request %s", err)
	}

	var headerBuffer []byte
	tempBuffer := make([]byte, 1024)
	contentLength := -1 // NOT INCLUDING BYTES ACCIDENTLY READ BY HEADER
	for {
		bytesGot, err := conn.Read(tempBuffer)
		if err != nil && err != io.EOF {
			return HttpResponse{}, fmt.Errorf("Failed receiving response%s", err)
		}

		isEof := err == io.EOF

		headerBuffer = append(headerBuffer, tempBuffer[:bytesGot]...)

		if bytes.Contains(headerBuffer, []byte("\r\n\r\n")) {
			headerEndIdx := bytes.Index(headerBuffer, []byte("\r\n\r\n"))
			headers := string(headerBuffer[:headerEndIdx])
			lines := strings.Split(headers, "\r\n")

			for _, line := range lines {
				if strings.HasPrefix(strings.ToLower(line), "content-length:") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						valStr := strings.TrimSpace(parts[1])
						contentLength, err = strconv.Atoi(valStr)
						if err != nil {
							return HttpResponse{}, fmt.Errorf("Failed parsing Content-Length: %s", err)
						}
						break
					}
				}
			}

			if contentLength == -1 {
				return HttpResponse{}, fmt.Errorf("Failed to find Content-Length header in response")
			}

			contentLength -= len(headerBuffer) - headerEndIdx - 4
			if contentLength < 0 {
				return HttpResponse{}, fmt.Errorf("Malformed response: captured more body bytes than Content-Length")
			}
			break
		}

		if isEof {
			return HttpResponse{}, fmt.Errorf("Connection closed unexpectedly")
		}
	}

	bodyBuffer := make([]byte, contentLength)
	_, err = io.ReadFull(conn, bodyBuffer)
	if err != nil {
		return HttpResponse{}, fmt.Errorf("Failed reading body: %w", err)
	}

	responseBytes := append(headerBuffer, bodyBuffer...)

	response, err := ResponseDecoder(responseBytes)
	if err != nil {
		return HttpResponse{}, fmt.Errorf("Failed parsing response %s", err)
	}

	return response, nil
}
