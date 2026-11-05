package main

import (
	"bytes"
	"compress/gzip"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"a03"
)

// Compile-time checks protect the starter's grader-facing signatures.
var (
	_ func(HttpRequest) ([]byte, error)                 = RequestEncoder
	_ func([]byte) (HttpResponse, error)                = ResponseDecoder
	_ func(HttpRequest, net.Conn) (HttpResponse, error) = Fetch
)

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	writer.Write(data)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// parseRequestWire is an independent fixture parser for encoded requests.
func parseRequestWire(t *testing.T, wire []byte) (string, a03.Headers) {
	t.Helper()
	text := string(wire)
	if !strings.HasSuffix(text, "\r\n\r\n") || strings.Count(text, "\r\n\r\n") != 1 {
		t.Fatalf("request must end with exactly one CRLF CRLF: %q", wire)
	}
	lines := strings.Split(strings.TrimSuffix(text, "\r\n\r\n"), "\r\n")
	headers := a03.Headers{}
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("invalid header line %q", line)
		}
		headers[strings.ToLower(name)] = strings.TrimSpace(value)
	}
	return lines[0], headers
}

func TestRequestEncoder(t *testing.T) {
	wire, err := RequestEncoder(HttpRequest{Method: "GET", URI: "/greet/2306123123?name=Rani", Version: a03.HTTPVersion,
		Headers: a03.Headers{"Host": "10.0.0.5:12345", "ACCEPT": "application/xml", "connection": "keep-alive"}})
	if err != nil {
		t.Fatal(err)
	}
	line, headers := parseRequestWire(t, wire)
	if line != "GET /greet/2306123123?name=Rani HTTP/1.1" {
		t.Fatalf("request line = %q", line)
	}
	if headers["host"] != "10.0.0.5:12345" || headers["accept"] != "application/xml" || headers["connection"] != "close" {
		t.Fatalf("want Host, Accept, and Connection: close, got %v", headers)
	}
	if _, ok := headers["accept-encoding"]; ok {
		t.Fatal("Accept-Encoding must be omitted unless set")
	}

	wire, err = RequestEncoder(HttpRequest{Method: "GET", URI: "/", Version: a03.HTTPVersion,
		Headers: a03.Headers{"host": "h:1", "accept": "*/*", "accept-encoding": "gzip"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, headers := parseRequestWire(t, wire); headers["accept-encoding"] != "gzip" || headers["connection"] != "close" {
		t.Fatalf("unexpected headers %v", headers)
	}

	for name, headers := range map[string]a03.Headers{
		"missing Host":    {"accept": "*/*"},
		"empty Host":      {"host": "", "accept": "*/*"},
		"missing Accept":  {"host": "h:1"},
		"CRLF in a value": {"host": "h:1", "accept": "*/*\r\nX-Injected: 1"},
		"colon in a name": {"host": "h:1", "accept": "*/*", "x:y": "z"},
	} {
		if _, err := RequestEncoder(HttpRequest{Method: "GET", URI: "/", Version: a03.HTTPVersion, Headers: headers}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := RequestEncoder(HttpRequest{Method: "GET", URI: "/a b", Version: a03.HTTPVersion,
		Headers: a03.Headers{"host": "h:1", "accept": "*/*"}}); err == nil {
		t.Error("whitespace in the URI must be rejected")
	}

	wire, err = RequestEncoder(HttpRequest{Method: "GET", URI: "/", Headers: a03.Headers{"host": "h:1", "accept": "*/*"}})
	if err != nil {
		t.Fatalf("an empty Version must encode as HTTP/1.1: %v", err)
	}
	if line, _ := parseRequestWire(t, wire); line != "GET / HTTP/1.1" {
		t.Fatalf("request line = %q", line)
	}
	for name, request := range map[string]HttpRequest{
		"other Version":    {Method: "GET", URI: "/", Version: "HTTP/1.0"},
		"empty Method":     {Method: "", URI: "/", Version: a03.HTTPVersion},
		"non-token Method": {Method: "G(T", URI: "/", Version: a03.HTTPVersion},
	} {
		request.Headers = a03.Headers{"host": "h:1", "accept": "*/*"}
		if _, err := RequestEncoder(request); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestResponseDecoderFragmentFixture(t *testing.T) {
	// This fixture is independent of the student server/encoder.
	wire := []byte("HTTP/1.1 200 OK\r\ncOnTeNt-LeNgTh: 5\r\nConnection: close\r\nContent-Type: text/plain\r\n\r\nhello")
	response, err := ResponseDecoder(wire)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || string(response.Body) != "hello" || response.Headers["content-length"] != "5" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestResponseDecoderGzip(t *testing.T) {
	payload := []byte(`{"Student":{"Nama":"Student","Npm":"2306123123"},"Greeter":"Rani"}`)
	compressed := gzipBytes(t, payload)
	wire := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Encoding: gzip\r\nConnection: close\r\nContent-Length: "+
		strconv.Itoa(len(compressed))+"\r\n\r\n"), compressed...)
	response, err := ResponseDecoder(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(response.Body, payload) {
		t.Fatalf("body = %q, want the decompressed payload", response.Body)
	}
	if response.Headers["content-length"] != strconv.Itoa(len(compressed)) {
		t.Fatalf("content-length = %q, want the wire length %d", response.Headers["content-length"], len(compressed))
	}
}

func TestResponseDecoderRejectsTruncatedBody(t *testing.T) {
	wire := []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nhi")
	if _, err := ResponseDecoder(wire); err == nil {
		t.Fatal("expected truncated-body error")
	}
}

func TestResponseDecoderRejectsInvalidMessages(t *testing.T) {
	const tail = "Content-Type: text/plain\r\nConnection: close\r\n"
	for name, wire := range map[string]string{
		"missing Content-Length":  "HTTP/1.1 200 OK\r\n" + tail + "\r\n",
		"negative Content-Length": "HTTP/1.1 200 OK\r\nContent-Length: -1\r\n" + tail + "\r\n",
		"nondecimal length":       "HTTP/1.1 200 OK\r\nContent-Length: 5a\r\n" + tail + "\r\nhello",
		"extra body bytes":        "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n" + tail + "\r\nhello",
		"truncated header":        "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n",
		"invalid status line":     "HTTP/1.1 OK\r\nContent-Length: 0\r\n" + tail + "\r\n",
		"invalid gzip":            "HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: 5\r\n" + tail + "\r\nhello",
	} {
		if _, err := ResponseDecoder([]byte(wire)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func readRequestHeader(conn net.Conn) bool {
	conn.SetDeadline(time.Now().Add(2 * a03.IOTimeout))
	var request []byte
	buf := make([]byte, 64)
	for !bytes.Contains(request, []byte("\r\n\r\n")) {
		n, err := conn.Read(buf)
		if err != nil {
			return false
		}
		request = append(request, buf[:n]...)
	}
	return true
}

// fakeServer reads one request header from conn, then writes the response as the
// given chunks. It deliberately keeps conn open until the test ends, so Fetch
// cannot rely on EOF to find the end of the response.
func fakeServer(t *testing.T, chunks ...string) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		client.Close()
	})
	go func() {
		defer server.Close()
		if !readRequestHeader(server) {
			return
		}
		for _, chunk := range chunks {
			if _, err := server.Write([]byte(chunk)); err != nil {
				return
			}
		}
		<-done
	}()
	return client
}

func TestFetchFraming(t *testing.T) {
	request := HttpRequest{Method: "GET", URI: "/", Version: a03.HTTPVersion, Headers: a03.Headers{"host": "h:1", "accept": "*/*"}}
	response := "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nConnection: close\r\nContent-Length: 5\r\n\r\nhello"
	var oneByte []string
	for i := range response {
		oneByte = append(oneByte, response[i:i+1])
	}
	split := strings.Index(response, "\r\n\r\n") + 2
	for name, chunks := range map[string][]string{
		"single write":       {response},
		"one byte at a time": oneByte,
		"split CRLF CRLF":    {response[:split], response[split:]},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Fetch(request, fakeServer(t, chunks...))
			if err != nil {
				t.Fatal(err)
			}
			if got.StatusCode != 200 || string(got.Body) != "hello" {
				t.Fatalf("unexpected response %+v", got)
			}
		})
	}
	t.Run("truncated body", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		go func() {
			defer server.Close()
			if readRequestHeader(server) {
				server.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nConnection: close\r\nContent-Length: 5\r\n\r\nhi"))
			}
		}()
		if _, err := Fetch(request, client); err == nil {
			t.Fatal("expected an error for a truncated body")
		}
	})
}
