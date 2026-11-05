package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"a03"
)

// Grader contract: retain these aliases and all exported function signatures.
type HttpRequest = a03.HttpRequest
type HttpResponse = a03.HttpResponse
type Student = a03.Student
type GreetResponse = a03.GreetResponse

// ServerStudent is configured before serving. Tests may replace it between cases.
// Do not change it while handling a connection.
var ServerStudent Student

// RequestDecoder validates a complete request header (including its terminator).
// Normalize header names to lowercase. Split header lines at the FIRST colon.
// Validate syntax and a nonempty Host before considering method or routing.
// Return an error for a header over 8192 bytes, a missing terminator, non-CRLF
// line endings, a request line that is not "<method> <origin-form URI> HTTP/1.1",
// a header line without a colon, or a missing/empty Host.
func RequestDecoder(message []byte) (HttpRequest, error) {
	// TODO D: validate HTTP/1.1 request syntax, CRLF, and the 8192-byte limit.
	// Do not reject syntactically valid methods here: HandleRequest produces 405.

	headerEndIdx := bytes.Index(message, []byte("\r\n\r\n"))
	if headerEndIdx == -1 {
		return HttpRequest{}, fmt.Errorf("Message doesn't contain CRLF CRLF")
	}

	if headerEndIdx+4 > a03.MaxRequestHeaderBytes {
		return HttpRequest{}, fmt.Errorf("Message header over 8192 bytes")
	}

	lines_str := string(message[:headerEndIdx])

	lines := strings.Split(lines_str, "\r\n")
	for _, line := range lines {
		if strings.ContainsAny(line, "\r\n") {
			return HttpRequest{}, fmt.Errorf("Message contains non-CRLF line endings")
		}
	}
	requestLine := strings.Split(lines[0], " ")
	if len(requestLine) != 3 {
		return HttpRequest{}, fmt.Errorf("Request line malformed")
	}

	method := requestLine[0]
	uri := requestLine[1]
	version := requestLine[2]

	if len(uri) == 0 || uri[0] != '/' {
		return HttpRequest{}, fmt.Errorf("URI is not in origin-form")
	}

	if strings.Compare(version, "HTTP/1.1") != 0 {
		return HttpRequest{}, fmt.Errorf("Server does not serve version other than HTTP/1.1")
	}

	headers_str := lines[1:]

	hostExist := false
	header_map := map[string]string{}
	for _, line := range headers_str {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			valStr := strings.TrimSpace(parts[1])
			keyStr := strings.ToLower(parts[0])
			if strings.ContainsAny(keyStr, " \t") {
				return HttpRequest{}, fmt.Errorf("Header name contains invalid whitespace")
			}
			header_map[keyStr] = valStr
			if keyStr == "host" {
				if valStr == "" {
					return HttpRequest{}, fmt.Errorf("Host header is empty")
				}
				hostExist = true
			}
		} else {
			return HttpRequest{}, fmt.Errorf("Header format malformed")
		}
	}

	if !hostExist {
		return HttpRequest{}, fmt.Errorf("Host header not found")
	}

	return HttpRequest{Method: method, URI: uri, Version: version, Headers: header_map}, nil
}

func ContainAllRune(s string, runs string) bool {
	for _, v := range s {
		if !strings.ContainsRune(runs, v) {
			return false
		}
	}
	return true
}

// HandleRequest applies status precedence: syntax/Host (400), method (405),
// route/NPM (404), Accept (406), then success (200). It must also handle manually
// constructed requests from unit tests: an empty Host, a Version other than
// HTTP/1.1, or an empty or non-token (RFC 9110) Method is 400. Use ServerStudent
// as the identity. Route on the path before "?"; a query on / is ignored.
func HandleRequest(request HttpRequest) HttpResponse {
	// TODO E: GET / returns the exact UTF-8 identity message without a newline.
	// TODO F: validate /greet/<10-digit NPM>, compare NPM, and read optional name.
	// TODO G: serialize JSON/XML using simplified Accept/q negotiation; JSON wins
	// ties. Only successful /greet responses may select gzip from Accept-Encoding.
	// Error bodies are empty text/plain; 405 also has Allow: GET. Content-Type
	// values are exact, without parameters such as charset.
	// Placeholder only: A03 never returns 501.

	uri_query := strings.SplitN(request.URI, "?", 2)
	host, host_found := request.Headers["host"]
	host_empty := host == ""
	versionNotSupported := request.Version != "HTTP/1.1"
	emptyMethod := len(request.Method) == 0
	nonTokenMethod := !ContainAllRune(request.Method, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&'*+-.^_`|~")
	if host_empty || !host_found || versionNotSupported || len(uri_query) < 1 || emptyMethod || nonTokenMethod {
		return HttpResponse{Version: a03.HTTPVersion, StatusCode: 400, Headers: a03.Headers{"content-type": "text/plain", "connection": "close"}}
	}
	uri := uri_query[0]
	if request.Method != "GET" {
		return HttpResponse{Version: a03.HTTPVersion, StatusCode: 405, Headers: a03.Headers{"content-type": "text/plain", "allow": "GET", "connection": "close"}}
	}

	if uri == "/" {
		return HttpResponse{
			Version:    a03.HTTPVersion,
			StatusCode: 200,
			Body:       []byte(fmt.Sprintf("Halo, dunia! Aku %s sedang mengerjakan A03", ServerStudent.Nama)),
			Headers:    a03.Headers{"content-type": "text/html", "connection": "close"},
		}
	} else if uri == fmt.Sprintf("/greet/%s", ServerStudent.Npm) {
		name := ServerStudent.Nama
		if len(uri_query) > 1 {
			query := uri_query[1]
			query_pairs := strings.Split(query, "&")
			for _, p := range query_pairs {
				key_val := strings.SplitN(p, "=", 2)
				if len(key_val) == 2 && key_val[0] == "name" && key_val[1] != "" {
					name = key_val[1]
				}
			}
		}

		qvals := make(map[string]float64)

		accept, ok := request.Headers["accept"]
		if accept == "" || !ok || accept == "*/*" {
			qvals["application/json"] = 1.0
		} else {
			contents_str := strings.Split(accept, ",")
			for _, c := range contents_str {
				types := strings.Split(c, ";")
				types[0] = strings.TrimSpace(types[0])
				if len(types) != 2 && len(types) != 1 {
					return HttpResponse{Version: a03.HTTPVersion, StatusCode: 406, Headers: a03.Headers{"content-type": "text/plain", "connection": "close"}}
				}
				if types[0] == "application/json" {
					if len(types) == 2 {
						valStr := strings.TrimSpace(types[1])
						valStr = strings.TrimPrefix(valStr, "q=")
						q, err := strconv.ParseFloat(valStr, 64)
						if err != nil && q > 0 {
							return HttpResponse{Version: a03.HTTPVersion, StatusCode: 406, Headers: a03.Headers{"content-type": "text/plain", "connection": "close"}}
						}
						qvals["application/json"] = q
					} else {
						qvals["application/json"] = 1.0
					}
				} else if types[0] == "application/xml" {
					if len(types) == 2 {
						valStr := strings.TrimSpace(types[1])
						valStr = strings.TrimPrefix(valStr, "q=")
						q, err := strconv.ParseFloat(valStr, 64)
						if err != nil && q > 0 {
							return HttpResponse{Version: a03.HTTPVersion, StatusCode: 406, Headers: a03.Headers{"content-type": "text/plain", "connection": "close"}}
						}
						qvals["application/xml"] = q
					} else {
						qvals["application/xml"] = 1.0
					}
				}
			}
		}
		if len(qvals) == 0 {
			return HttpResponse{Version: a03.HTTPVersion, StatusCode: 406, Headers: a03.Headers{"content-type": "text/plain", "connection": "close"}}
		}

		content_chosen := "application/json"

		allZero := true

		for k, v := range qvals {
			if qvals[content_chosen] < v {
				content_chosen = k
			}
			if qvals[content_chosen] != 0.0 {
				allZero = false
			}
		}

		if allZero {
			return HttpResponse{Version: a03.HTTPVersion, StatusCode: 406, Headers: a03.Headers{"content-type": "text/plain", "connection": "close"}}
		}

		headers := a03.Headers{"connection": "close"}

		accept_encoding, ok := request.Headers["accept-encoding"]
		if ok {
			encodings := strings.Split(accept_encoding, ",")
			for _, e := range encodings {
				if strings.EqualFold(strings.TrimSpace(e), "gzip") {
					headers["content-encoding"] = "gzip"
					break
				}
			}
		}

		headers["content-type"] = content_chosen

		greetResponse := GreetResponse{Student: Student{Nama: ServerStudent.Nama, Npm: ServerStudent.Npm}, Greeter: name}
		body, _ := json.Marshal(greetResponse)

		if content_chosen == "application/xml" {
			body, _ = xml.Marshal(greetResponse)
		}
		return HttpResponse{
			Version:    a03.HTTPVersion,
			StatusCode: 200,
			Headers:    headers,
			Body:       body,
		}

	} else {
		return HttpResponse{Version: a03.HTTPVersion, StatusCode: 404, Headers: a03.Headers{"content-type": "text/plain", "connection": "close"}}
	}
}

// ResponseEncoder serializes a response using standard reason phrases and CRLF.
// Always compute Content-Length from the final wire bytes. Compress Body first
// when content-encoding is gzip and the status is 200; otherwise omit
// Content-Encoding. Always send Connection: close. Return an error for a status
// other than 200, 400, 404, 405, or 406, a missing Content-Type, or a Version
// other than empty or HTTP/1.1. Always write HTTP/1.1 on the status line.
func ResponseEncoder(response HttpResponse) ([]byte, error) {
	contentTypeFound := false
	for k, v := range response.Headers {
		if strings.EqualFold(k, "content-type") && v != "" {
			contentTypeFound = true
		}
		if strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("CR or LF in header name or value")
		}
		// if strings.ContainsAny(k, ": \t") {
		// 	return nil, fmt.Errorf("Colon or white space in header name")
		// }
	}

	if !contentTypeFound {
		return nil, fmt.Errorf("Content-Type missing")
	}

	validCodes := []int{200, 400, 404, 405, 406}
	defaultReasons := map[int]string{200: "OK", 400: "Bad Request", 404: "Not Found", 405: "Method Not Allowed", 406: "Not Acceptable"}

	if !slices.Contains(validCodes, response.StatusCode) {
		return nil, fmt.Errorf("Status code not supported")
	}

	if len(response.Version) != 0 && strings.Compare(response.Version, "HTTP/1.1") != 0 {
		return nil, fmt.Errorf("Version is not empty or exactly HTTP/1.1")
	}

	result := []byte("HTTP/1.1")
	result = append(result, ' ')
	result = append(result, []byte(strconv.Itoa(response.StatusCode))...)
	result = append(result, ' ')
	result = append(result, []byte(defaultReasons[response.StatusCode])...)
	result = append(result, []byte("\r\n")...)

	useGzip := false
	for k, v := range response.Headers {
		if strings.EqualFold(k, "content-encoding") {
			if strings.EqualFold(v, "gzip") && response.StatusCode == 200 {
				useGzip = true
			} else {
				continue
			}
		}

		result = append(result, []byte(k)...)
		result = append(result, []byte(": ")...)
		result = append(result, []byte(v)...)
		result = append(result, []byte("\r\n")...)
	}

	var finalBody []byte
	if useGzip {
		var b bytes.Buffer
		gzipWriter := gzip.NewWriter(&b)
		gzipWriter.Write(response.Body)
		gzipWriter.Close()
		finalBody = append(finalBody, b.Bytes()...)
	} else {
		finalBody = append(finalBody, response.Body...)
	}

	result = append(result, []byte("content-length: "+strconv.Itoa(len(finalBody))+"\r\n")...)
	result = append(result, []byte("connection: close\r\n")...)

	result = append(result, []byte("\r\n")...)

	result = append(result, finalBody...)
	return result, nil
}

// HandleConnection serves exactly one request and one response, using the
// decoder, handler, and encoder above. The caller owns and closes conn.
func HandleConnection(conn net.Conn) error {
	// TODO I: set a finite deadline, accumulate headers up to 8192 bytes including
	// CRLF CRLF, decode, handle, encode, and write all response bytes.
	// Malformed/oversized input gets 400 if possible. A disconnected peer can be
	// closed without a response. Never panic or stop the outer accept loop.
	if err := conn.SetDeadline(time.Now().Add(a03.IOTimeout)); err != nil {
		return fmt.Errorf("deadline: %w", err)
	}

	// Read request
	var headerBuffer []byte
	tempBuffer := make([]byte, 1024)
	for {

		bytesGot, err := conn.Read(tempBuffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("Failed receiving response%s", err)
		}

		isEof := err == io.EOF

		headerBuffer = append(headerBuffer, tempBuffer[:bytesGot]...)

		if len(headerBuffer) > a03.MaxRequestHeaderBytes {
			errorResponse := HttpResponse{
				Version:    a03.HTTPVersion,
				StatusCode: 400,
				Headers:    a03.Headers{"content-type": "text/plain", "connection": "close"},
			}
			encodedErr, _ := ResponseEncoder(errorResponse)
			conn.Write(encodedErr)
			return fmt.Errorf("bad request: %v", err)
		}

		if bytes.Contains(headerBuffer, []byte("\r\n\r\n")) {
			break
		}

		if isEof {
			return fmt.Errorf("Connection closed unexpectedly")
		}
	}

	request, err := RequestDecoder(headerBuffer)
	if err != nil {
		errorResponse := HttpResponse{
			Version:    a03.HTTPVersion,
			StatusCode: 400,
			Headers:    a03.Headers{"content-type": "text/plain", "connection": "close"},
		}
		encodedErr, _ := ResponseEncoder(errorResponse)
		conn.Write(encodedErr)
		return fmt.Errorf("bad request: %v", err)
	}

	response := HandleRequest(request)

	response_bytes, err := ResponseEncoder(response)
	if err != nil {
		return fmt.Errorf("Server error failed encoding response %s", err)
	}

	_, err = conn.Write(response_bytes)
	if err != nil {
		return fmt.Errorf("Failed sending response %s", err)
	}

	return nil
}
