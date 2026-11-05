package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"a03"
)

var (
	_ func([]byte) (HttpRequest, error)  = RequestDecoder
	_ func(HttpResponse) ([]byte, error) = ResponseEncoder
	_ func(HttpRequest) HttpResponse     = HandleRequest
	_ func(net.Conn) error               = HandleConnection
)

const testNPM = "2406358056"

func useTestStudent(t *testing.T) {
	t.Helper()
	previous := ServerStudent
	ServerStudent = Student{Nama: "Student", Npm: testNPM}
	t.Cleanup(func() { ServerStudent = previous })
}

// parseWire is an independent fixture parser, so encoder tests do not depend on
// a student's own decoder.
func parseWire(t *testing.T, wire []byte) (int, a03.Headers, []byte) {
	t.Helper()
	head, body, ok := bytes.Cut(wire, []byte("\r\n\r\n"))
	if !ok {
		t.Fatalf("response has no CRLF CRLF terminator: %q", wire)
	}
	lines := strings.Split(string(head), "\r\n")
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) != 3 || parts[0] != "HTTP/1.1" {
		t.Fatalf("invalid status line %q", lines[0])
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("invalid status code in %q", lines[0])
	}
	reasons := map[int]string{200: "OK", 400: "Bad Request", 404: "Not Found", 405: "Method Not Allowed", 406: "Not Acceptable"}
	if parts[2] != reasons[status] {
		t.Fatalf("reason phrase %q, want %q", parts[2], reasons[status])
	}
	headers := a03.Headers{}
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("invalid header line %q", line)
		}
		headers[strings.ToLower(name)] = strings.TrimSpace(value)
	}
	if headers["content-length"] != strconv.Itoa(len(body)) {
		t.Fatalf("content-length %q, wire body has %d bytes", headers["content-length"], len(body))
	}
	if headers["connection"] != "close" || headers["content-type"] == "" {
		t.Fatalf("missing required headers: %v", headers)
	}
	return status, headers, body
}

func gunzip(t *testing.T, data []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("invalid gzip: %v", err)
	}
	plain, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("invalid gzip: %v", err)
	}
	return plain
}

func TestHandlerStatusPrecedence(t *testing.T) {
	useTestStudent(t)
	cases := []struct {
		name, method, uri, version, host, accept string
		want                                     int
	}{
		{"health ignores Accept", "GET", "/", "HTTP/1.1", "localhost:12345", "text/plain", 200},
		{"missing Host before method", "POST", "/missing", "HTTP/1.1", "", "text/plain", 400},
		{"wrong version", "GET", "/", "HTTP/1.0", "localhost:12345", "", 400},
		{"empty method", "", "/", "HTTP/1.1", "localhost:12345", "", 400},
		{"non-token method", "G(T", "/", "HTTP/1.1", "localhost:12345", "", 400},
		{"method before route", "POST", "/missing", "HTTP/1.1", "localhost:12345", "text/plain", 405},
		{"route before Accept", "GET", "/greet/9999999999", "HTTP/1.1", "localhost:12345", "text/plain", 404},
		{"unsupported representation", "GET", "/greet/" + testNPM, "HTTP/1.1", "localhost:12345", "text/plain", 406},
		{"default JSON", "GET", "/greet/" + testNPM, "HTTP/1.1", "localhost:12345", "", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := HandleRequest(HttpRequest{Method: tc.method, URI: tc.uri, Version: tc.version, Headers: a03.Headers{"host": tc.host, "accept": tc.accept}})
			if response.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.want)
			}
			if tc.want >= 400 {
				if len(response.Body) != 0 || response.Headers["content-type"] != "text/plain" || response.Headers["content-encoding"] != "" {
					t.Fatalf("invalid error response: %+v", response)
				}
				if tc.want == 405 && response.Headers["allow"] != "GET" {
					t.Fatal("405 must include Allow: GET")
				}
			}
		})
	}
}

func TestHandlerHealth(t *testing.T) {
	useTestStudent(t)
	for _, uri := range []string{"/", "/?x=1"} {
		response := HandleRequest(HttpRequest{Method: "GET", URI: uri, Version: "HTTP/1.1",
			Headers: a03.Headers{"host": "h:1", "accept": "application/xml", "accept-encoding": "gzip"}})
		if response.StatusCode != 200 || response.Headers["content-type"] != "text/html" || response.Headers["content-encoding"] != "" {
			t.Fatalf("%s: unexpected health response: %+v", uri, response)
		}
		if want := "Halo, dunia! Aku Student sedang mengerjakan A03"; string(response.Body) != want {
			t.Fatalf("%s: body = %q, want %q", uri, response.Body, want)
		}
	}
}

func TestHandlerRoutes(t *testing.T) {
	useTestStudent(t)
	notFound := []string{
		"/lainnya", "/greet", "/greet/", "/greet/9999999999", "/greet/230612312",
		"/greet/23061231234", "/greet/23061231x3", "/greet/" + testNPM + "/x", "/greet/" + testNPM + "x",
	}
	for _, uri := range notFound {
		response := HandleRequest(HttpRequest{Method: "GET", URI: uri, Version: "HTTP/1.1",
			Headers: a03.Headers{"host": "h:1", "accept": "text/plain"}})
		if response.StatusCode != 404 || len(response.Body) != 0 {
			t.Errorf("%s: status = %d, want empty 404", uri, response.StatusCode)
		}
	}
}

func TestHandlerNegotiation(t *testing.T) {
	useTestStudent(t)
	cases := []struct {
		name, query, accept, encoding string
		omitAccept                    bool
		wantType, wantGreeter         string
		wantGzip                      bool
	}{
		{name: "absent Accept", omitAccept: true, wantType: "application/json", wantGreeter: "Student"},
		{name: "empty Accept", wantType: "application/json", wantGreeter: "Student"},
		{name: "wildcard", accept: "*/*", wantType: "application/json", wantGreeter: "Student"},
		{name: "XML with name", query: "?name=Rani", accept: "application/xml", wantType: "application/xml", wantGreeter: "Rani"},
		{name: "empty name", query: "?name=", accept: "application/json", wantType: "application/json", wantGreeter: "Student"},
		{name: "other query ignored", query: "?x=1&name=a_b-9", accept: "application/json", wantType: "application/json", wantGreeter: "a_b-9"},
		{name: "higher q", accept: "application/xml;q=0.8, application/json;q=0.9", wantType: "application/json", wantGreeter: "Student"},
		{name: "tie picks JSON", accept: "application/xml;q=0.9, application/json;q=0.9", wantType: "application/json", wantGreeter: "Student"},
		{name: "three decimals and spaces", accept: "application/json ; q=0.5 ,  application/xml;q=0.501", wantType: "application/xml", wantGreeter: "Student"},
		{name: "unsupported mixed in", accept: "text/html, application/xml;q=0.2", wantType: "application/xml", wantGreeter: "Student"},
		{name: "q=0 excluded", accept: "application/json;q=0, application/xml", wantType: "application/xml", wantGreeter: "Student"},
		{name: "gzip", accept: "application/json", encoding: "gzip", wantType: "application/json", wantGreeter: "Student", wantGzip: true},
		{name: "gzip mixed case with identity", accept: "application/json", encoding: "identity, GZip", wantType: "application/json", wantGreeter: "Student", wantGzip: true},
		{name: "identity", accept: "application/json", encoding: "identity", wantType: "application/json", wantGreeter: "Student"},
		{name: "unsupported encoding", accept: "application/json", encoding: "br", wantType: "application/json", wantGreeter: "Student"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := a03.Headers{"host": "h:1"}
			if !tc.omitAccept {
				headers["accept"] = tc.accept
			}
			if tc.encoding != "" {
				headers["accept-encoding"] = tc.encoding
			}
			response := HandleRequest(HttpRequest{Method: "GET", URI: "/greet/" + testNPM + tc.query, Version: "HTTP/1.1", Headers: headers})
			if response.StatusCode != 200 || response.Headers["content-type"] != tc.wantType {
				t.Fatalf("status/type = %d %q, want 200 %q", response.StatusCode, response.Headers["content-type"], tc.wantType)
			}
			if gotGzip := response.Headers["content-encoding"] == "gzip"; gotGzip != tc.wantGzip {
				t.Fatalf("content-encoding = %q, want gzip=%v", response.Headers["content-encoding"], tc.wantGzip)
			}
			// HandleRequest supplies the uncompressed body; ResponseEncoder compresses it.
			var got struct {
				XMLName xml.Name
				Student Student
				Greeter string
			}
			if tc.wantType == "application/json" {
				if err := json.Unmarshal(response.Body, &got); err != nil {
					t.Fatalf("invalid JSON %q: %v", response.Body, err)
				}
			} else {
				if err := xml.Unmarshal(response.Body, &got); err != nil {
					t.Fatalf("invalid XML %q: %v", response.Body, err)
				}
				if got.XMLName.Local != "GreetResponse" {
					t.Fatalf("XML root = %q, want GreetResponse", got.XMLName.Local)
				}
			}
			if got.Student != ServerStudent || got.Greeter != tc.wantGreeter {
				t.Fatalf("payload = %+v, want student %+v and greeter %q", got, ServerStudent, tc.wantGreeter)
			}
		})
	}
	for _, accept := range []string{"text/plain", "application/json;q=0", "application/json;q=0, application/xml;q=0"} {
		response := HandleRequest(HttpRequest{Method: "GET", URI: "/greet/" + testNPM, Version: "HTTP/1.1",
			Headers: a03.Headers{"host": "h:1", "accept": accept, "accept-encoding": "gzip"}})
		if response.StatusCode != 406 || len(response.Body) != 0 || response.Headers["content-encoding"] != "" {
			t.Errorf("Accept %q: got %+v, want empty 406 without gzip", accept, response)
		}
	}
}

func TestRequestDecoder(t *testing.T) {
	request, err := RequestDecoder([]byte("GET /greet/" + testNPM + "?name=Rani HTTP/1.1\r\nX-Unknown: ignored\r\naCCepT:  application/xml \t\r\nHOST: 10.0.0.5:12345\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != "GET" || request.URI != "/greet/"+testNPM+"?name=Rani" || request.Version != "HTTP/1.1" {
		t.Fatalf("unexpected request line fields: %+v", request)
	}
	if request.Headers["host"] != "10.0.0.5:12345" || request.Headers["accept"] != "application/xml" {
		t.Fatalf("headers must be lowercase, split at the first colon, and trimmed: %v", request.Headers)
	}
	if request, err := RequestDecoder([]byte("POST / HTTP/1.1\r\nHost: h\r\n\r\n")); err != nil || request.Method != "POST" {
		t.Fatalf("a valid non-GET request must decode for the handler's 405: %+v, %v", request, err)
	}

	prefix := "GET / HTTP/1.1\r\nHost: h\r\nX: "
	exact := prefix + strings.Repeat("a", a03.MaxRequestHeaderBytes-len(prefix)-4) + "\r\n\r\n"
	if _, err := RequestDecoder([]byte(exact)); err != nil {
		t.Fatalf("an 8192-byte header must be accepted: %v", err)
	}
	invalid := map[string]string{
		"8193 bytes":           prefix + strings.Repeat("a", a03.MaxRequestHeaderBytes-len(prefix)-3) + "\r\n\r\n",
		"no terminator":        "GET / HTTP/1.1\r\nHost: h\r\n",
		"LF line endings":      "GET / HTTP/1.1\nHost: h\n\n",
		"two-part line":        "GET /\r\nHost: h\r\n\r\n",
		"wrong version":        "GET / HTTP/1.0\r\nHost: h\r\n\r\n",
		"header without colon": "GET / HTTP/1.1\r\nHost h\r\n\r\n",
		"missing Host":         "GET / HTTP/1.1\r\nAccept: */*\r\n\r\n",
		"empty Host":           "GET / HTTP/1.1\r\nHost:  \r\n\r\n",
		"empty message":        "",
	}
	for name, message := range invalid {
		if _, err := RequestDecoder([]byte(message)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestResponseEncoder(t *testing.T) {
	wire, err := ResponseEncoder(HttpResponse{Version: "HTTP/1.1", StatusCode: 405,
		Headers: a03.Headers{"Content-Type": "text/plain", "connection": "close", "allow": "GET", "content-length": "99"}})
	if err != nil {
		t.Fatal(err)
	}
	if status, headers, body := parseWire(t, wire); status != 405 || headers["allow"] != "GET" || len(body) != 0 {
		t.Fatalf("unexpected 405: %q", wire)
	}

	// Content-Length counts bytes, not characters: "é" is two bytes in UTF-8.
	wire, err = ResponseEncoder(HttpResponse{Version: "HTTP/1.1", StatusCode: 200,
		Headers: a03.Headers{"content-type": "text/html"}, Body: []byte("é")})
	if err != nil {
		t.Fatal(err)
	}
	if _, headers, _ := parseWire(t, wire); headers["content-length"] != "2" || headers["connection"] != "close" {
		t.Fatalf("expected byte length 2 and Connection: close, got %v", headers)
	}

	payload := []byte(`{"Student":{"Nama":"Student","Npm":"2306123123"},"Greeter":"Student"}`)
	wire, err = ResponseEncoder(HttpResponse{Version: "HTTP/1.1", StatusCode: 200,
		Headers: a03.Headers{"content-type": "application/json", "connection": "close", "content-encoding": "gzip"}, Body: payload})
	if err != nil {
		t.Fatal(err)
	}
	_, headers, body := parseWire(t, wire)
	if headers["content-encoding"] != "gzip" || !bytes.Equal(gunzip(t, body), payload) {
		t.Fatalf("gzip response did not round-trip: %v", headers)
	}

	wire, err = ResponseEncoder(HttpResponse{Version: "HTTP/1.1", StatusCode: 404,
		Headers: a03.Headers{"content-type": "text/plain", "connection": "close", "content-encoding": "gzip"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, headers, body := parseWire(t, wire); headers["content-encoding"] != "" || len(body) != 0 {
		t.Fatalf("error responses must not be gzipped: %q", wire)
	}

	// An empty Version encodes as HTTP/1.1; parseWire checks the status line.
	wire, err = ResponseEncoder(HttpResponse{StatusCode: 404, Headers: a03.Headers{"content-type": "text/plain"}})
	if err != nil {
		t.Fatalf("an empty Version must encode as HTTP/1.1: %v", err)
	}
	parseWire(t, wire)

	for name, response := range map[string]HttpResponse{
		"unsupported status":   {Version: "HTTP/1.1", StatusCode: 501, Headers: a03.Headers{"content-type": "text/plain"}},
		"missing Content-Type": {Version: "HTTP/1.1", StatusCode: 200, Headers: a03.Headers{}},
		"other Version":        {Version: "HTTP/1.0", StatusCode: 200, Headers: a03.Headers{"content-type": "text/plain"}},
	} {
		if _, err := ResponseEncoder(response); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// exchange runs HandleConnection against a net.Pipe peer, writes the request
// as the given chunks, and returns everything the server wrote.
func exchange(t *testing.T, chunks ...string) ([]byte, error) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close() })
	result := make(chan error, 1)
	go func() {
		result <- HandleConnection(server)
		server.Close()
	}()
	go func() {
		for _, chunk := range chunks {
			if _, err := client.Write([]byte(chunk)); err != nil {
				return // The server may answer (400) and close before reading everything.
			}
		}
	}()
	client.SetDeadline(time.Now().Add(2 * a03.IOTimeout))
	wire, _ := io.ReadAll(client)
	select {
	case err := <-result:
		return wire, err
	case <-time.After(2 * a03.IOTimeout):
		t.Fatal("HandleConnection did not return")
		return nil, nil
	}
}

func TestHandleConnectionFraming(t *testing.T) {
	useTestStudent(t)
	request := "GET /greet/" + testNPM + " HTTP/1.1\r\nHost: h:1\r\nAccept: application/json\r\nAccept-Encoding: gzip\r\n\r\n"
	var oneByte []string
	for i := range request {
		oneByte = append(oneByte, request[i:i+1])
	}
	for name, chunks := range map[string][]string{
		"single write":       {request},
		"one byte at a time": oneByte,
		"split CRLF CRLF":    {request[:len(request)-3], request[len(request)-3 : len(request)-1], request[len(request)-1:]},
	} {
		t.Run(name, func(t *testing.T) {
			wire, err := exchange(t, chunks...)
			if err != nil {
				t.Fatal(err)
			}
			status, headers, body := parseWire(t, wire)
			var got GreetResponse
			if status != 200 || headers["content-encoding"] != "gzip" || json.Unmarshal(gunzip(t, body), &got) != nil || got.Student != ServerStudent {
				t.Fatalf("unexpected response %q", wire)
			}
		})
	}
}

func TestHandleConnectionErrors(t *testing.T) {
	useTestStudent(t)
	for name, request := range map[string]string{
		"malformed":         "GARBAGE\r\n\r\n",
		"missing Host":      "GET / HTTP/1.1\r\n\r\n",
		"unterminated 8193": "GET / HTTP/1.1\r\nX: " + strings.Repeat("a", a03.MaxRequestHeaderBytes),
	} {
		t.Run(name, func(t *testing.T) {
			wire, _ := exchange(t, request)
			if status, _, body := parseWire(t, wire); status != 400 || len(body) != 0 {
				t.Fatalf("want empty 400, got %q", wire)
			}
		})
	}
	t.Run("peer disconnects early", func(t *testing.T) {
		client, server := net.Pipe()
		go func() {
			client.Write([]byte("GET / HTTP/1.1\r\nHo"))
			client.Close()
		}()
		if err := HandleConnection(server); err == nil {
			t.Fatal("expected an error for an incomplete request")
		}
		server.Close()
	})
	t.Run("stalled peer times out", func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		done := make(chan error, 1)
		go func() { done <- HandleConnection(server) }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("expected a timeout error")
			}
		case <-time.After(2 * a03.IOTimeout):
			t.Fatal("HandleConnection must set its own finite deadline")
		}
	})
}
