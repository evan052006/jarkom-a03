# Starter interface contract

Keep one module named `a03`. The executable packages are `./client` and `./server`
(both `package main`). Shared types live in the root `a03` package; executable
packages expose aliases so grader tests can use the names below directly.
Tests for unexported package-main behavior can be placed in the corresponding
client/server directory using `package main`.

```go
// client
func RequestEncoder(request HttpRequest) ([]byte, error)
func ResponseDecoder(message []byte) (HttpResponse, error)
func Fetch(request HttpRequest, conn net.Conn) (HttpResponse, error)

// server
func RequestDecoder(message []byte) (HttpRequest, error)
func HandleRequest(request HttpRequest) HttpResponse
func ResponseEncoder(response HttpResponse) ([]byte, error)
func HandleConnection(conn net.Conn) error
```

Do not change these signatures or the shared fields in `types.go`. Add helper
functions/files as needed. No supplied exercise signature adds an identity
argument: `server.ServerStudent` is set from `-name` and `-npm` before the accept
loop. Tests can set it before calling HandleRequest and restore it afterward;
identity-changing tests must not run in parallel.

`HttpRequest` has `Method`, `URI`, `Version` strings and `Headers` (a map of
string to string). `HttpResponse` has `Version`, numeric `StatusCode`, `Headers`,
and `Body []byte`. There is no request body. A decoded URI retains its query.

Both encoders always write `HTTP/1.1` on the start line. An empty `Version` is
treated as `HTTP/1.1`, so a struct built without it still encodes; any other
`Version` is an error. A syntactically valid method is an RFC 9110 token: one or
more letters, digits, or ``!#$%&'*+-.^_`|~`` characters.

Decoded header keys must be lowercase. Encoders must accept header names in any
capitalization, though callers should normally use lowercase. Header order does
not matter. Unknown headers must be ignored. Split on the first colon,
trim horizontal whitespace around values, and compare names case-insensitively.
Duplicate headers are outside grader inputs; callers should avoid constructing
a map with differently capitalized spellings of the same name.

RequestEncoder encodes the supplied headers and always sends `Connection: close`,
replacing any supplied value. It sends Accept-Encoding only when the caller set
it. It returns an error for a missing/empty Host or Accept, CR or LF in a header
name or value, a colon or whitespace in a header name, whitespace in the URI, or
an empty or non-token Method.

ResponseEncoder owns Content-Length calculation and overrides stale lengths.
It always sends `Connection: close` and returns an error for a status other than
200, 400, 404, 405, or 406, or a missing Content-Type. HandleRequest supplies an
uncompressed body, selects its Content-Type, and sets Content-Encoding to gzip
only when needed. The encoder compresses it once, and only for status 200; it
omits Content-Encoding on every other response.
ResponseDecoder checks the wire length before decompression, returns the decoded
body, and retains Content-Length from the wire. Identity omits Content-Encoding.
Do not confuse byte length with Unicode character count.

Fetch and HandleConnection use finite deadlines even when invoked directly.
Their callers own connection closure. A successful Fetch sends one request and
consumes one framed response; HandleConnection sends one response. Neither should
rely on EOF as successful response framing. Return errors for I/O/codec failures;
HandleConnection should send a valid 400 for malformed requests when possible.

Syntax/Host validation precedes method, route, and negotiation. A decoder must
allow syntactically valid non-GET methods through for the handler's 405 response.
The handler also checks requests constructed directly by tests: an empty Host, a
Version other than `HTTP/1.1`, or an empty or non-token Method gives 400. All error
responses use text/plain, an empty body, no gzip, and Connection: close; 405 adds
Allow: GET. The encoder supplies Content-Length: 0.

Routing uses the path before `?`. A query on `/` is ignored, so `GET /?x=1`
returns the health response. Server Content-Type values are exactly `text/html`,
`application/json`, `application/xml`, or `text/plain`, with no parameters such
as `charset`.
