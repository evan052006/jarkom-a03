# A03 TCP and HTTP/1.1 student starter

A buildable Go scaffold for Assignment 03, specification version 3
(20261026-1015). Students implement HTTP messages and TCP stream handling using
Go's standard library.

The specification, [A03-webserver-spec-v3.docx](A03-webserver-spec-v3.docx),
defines the required behavior. This starter defines the Go names, fields, and
signatures that the grader uses; see [the interface contract](docs/INTERFACES.md).

## Prerequisites and commands

Use **Go 1.27.0**, the course's Ubuntu 26.04 LTS environment, and your unique A01a
port. There are no external dependencies, nested modules, or `go.work` files.
Do not import `net/http`, `net/http/httputil`, or HTTP libraries/frameworks.

```sh
go version
go build ./...
go test -v ./...
go vet ./...
```

Replace the example name, NPM, and port with your own. Run the server and
client in separate terminals (and eventually on separate AWS instances):

```sh
go run ./server -port 12345 -name 'Nama Anda' -npm 2306123123
go run ./client -host 127.0.0.1 -port 12345 -uri /
go run ./client -host 127.0.0.1 -port 12345 \
  -uri '/greet/2306123123?name=Rani' -accept application/xml
go run ./client -host 127.0.0.1 -port 12345 \
  -uri /greet/2306123123 \
  -accept 'application/xml;q=0.8, application/json;q=0.9' -encoding gzip
```

Initially the server listens and closes accepted connections with an
`exercise not implemented` log; the client reports an unfinished Fetch exercise.
The handler's 501 placeholder is not an A03 response and must be replaced.
The behavior tests are skipped: remove each `t.Skip` as you finish the
matching exercise. The automated grader is not distributed.

## Exercise sequence

1. Read [the interface contract](docs/INTERFACES.md) and implement the request
   encoder and request decoder (TODO A/D).
2. Implement the health handler and response encoder (TODO E/H). The root body
   is exactly `Halo, dunia! Aku <NAMA_ANDA> sedang mengerjakan A03`, without a
   trailing newline.
3. Complete Fetch and HandleConnection (TODO C/I). Handle partial reads/writes,
   deadlines, connection closure, the header limit, and malformed input.
4. Implement response decoding (TODO B), then the NPM route and query (TODO F).
5. Add JSON/XML negotiation (TODO G), then gzip in the handler and codecs.
6. Enable and extend the supplied tests. Work through
   [the test checklist](docs/TESTING.md), then test your server with raw
   requests from `nc` (see T08).
7. Run the two-instance AWS demonstration and collect T00–T08 evidence.

The CLI already provides TCP dialing/listening, configuration validation, finite
socket deadlines, connection cleanup, and output formatting. Each networking
exercise must also set deadlines itself so direct grader calls are bounded.
Server execution is sequential; concurrency is optional.

## Layout

| Path | Purpose |
| --- | --- |
| `types.go` | Shared request/response types, identity types, constants |
| `client/main.go` | Client flags, dial, and decoded output |
| `client/protocol.go` | Student request encoder, response decoder, Fetch |
| `server/main.go` | Server flags, identity configuration, accept loop |
| `server/protocol.go` | Student decoder, handler, response encoder, connection handling |
| `client/protocol_test.go`, `server/protocol_test.go` | Signature checks and skipped exercise tests |
| `docs/` | Interface contract and testing roadmap (including the report evidence checklist) |

## Submission and attribution

Submit `A03_[NPM].zip` containing the source/module and `A03_[NPM].pdf` containing
the report, as required by the course. Exclude binaries, caches, generated
artifacts, and the specification `.docx`. Acknowledge AI assistance and other
external sources according to the specification.
