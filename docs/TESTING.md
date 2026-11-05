# Testing and evidence checklist

The supplied behavioral tests are skipped. Remove each skip when
implementing that feature. They cover most of the checks below with independent
fixtures; add your own cases instead of relying only on round trips between your
own client and server. `go test -v ./...` shows skips.
Compilation and signature checks do not establish protocol correctness.

## Local protocol checks

- Request encoding: exact CRLF terminator, Host with port, Accept, Connection:
  close; omit Accept-Encoding for none/identity and include it for gzip. An empty
  Version encodes as HTTP/1.1; another Version or a non-token Method fails.
- Request decoding: mixed-case/shuffled headers, Host with colon, unknown headers,
  whitespace around values; malformed lines and absent/empty Host fail.
- Header limits: exactly 8192 bytes including terminator is permitted; 8193 is
  rejected with 400. Test a long unterminated header too.
- Response encoding: required headers, standard reason phrase, byte-based wire
  Content-Length, 405 Allow: GET, empty error bodies and no error gzip. Content-Type
  values carry no parameters; an empty Version encodes as HTTP/1.1.
- Response decoding: missing/negative/nondecimal Content-Length, truncated header,
  truncated body, invalid status line, invalid gzip, and length mismatches fail.
- Health: exact configured name and body without newline; ignore Accept, gzip,
  and any query.
- Route: configured 10-digit NPM succeeds; another NPM, malformed digits, suffix,
  and other paths produce 404 before representation negotiation.
- Greeter: default configured name, empty name, and an unreserved query value.
  JSON has Student/Nama/Npm and Greeter; XML has root GreetResponse.
- Accept: absent/empty/*/* defaults JSON; positive highest q wins; ties select
  JSON; q=0 is excluded; unsupported types alone yield 406. Include whitespace,
  unsupported types mixed with supported types, and q with three decimal places.
- Accept-Encoding: absent/empty/identity/br gives identity; gzip and
  `identity, GZip` give gzip. Check decompressed payload and compressed wire length.
- Precedence: malformed/Host -> 400, non-GET -> 405, unknown route -> 404,
  unacceptable representation on a valid greet route -> 406.

## TCP checks

Use `net.Pipe` or a loopback listener and an independent fixture peer. Split the
header terminator across writes, send a response one byte at a time, and combine
header/body in one write. Confirm buffered body bytes survive header parsing.
Bound test peers with deadlines and close them during test cleanup.

Disconnect before request completion, truncate a response body, and stall a peer
to exercise timeout/error paths. Then make another server connection to confirm
the accept loop survives. Also send hand-written requests from an independent
tool such as `nc`, not only from your own client. Full HTTP, TLS, chunked bodies,
persistent connections, concurrency, and duplicate headers are outside A03.

## Report evidence

Use the assigned port consistently. For remote tests, replace the client's host
with the server instance address and permit that port through AWS security groups
and host firewalls. The server defaults to 0.0.0.0 for remote IPv4 access.

| Code | Evidence to collect |
| --- | --- |
| T00 | Go 1.27.0 version and successful root build |
| T01 | Remote GET /, 200, text/html, exact identity body |
| T02 | Remote greet JSON and correct data |
| T03 | Remote greet XML and correct data |
| T04 | Remote multiple Accept values with q-based selection |
| T05 | Remote gzip header, wire length, decompressed body |
| T06 | Remote /lainnya returns empty 404; server continues |
| T07 | `go test -v ./...` with every `t.Skip` removed; unedited output |
| T08 | Raw `nc` requests from the client instance: `POST /` gives 405 with `Allow: GET`, a malformed request gives 400, then `GET /` gives 200 |

For each test, capture invocation, output/status, host/IP context, and port. Write
2–4 sentences describing the input, observed result, and relevant rule. T01–T06
and T08 must use the two AWS instances. Report PASS and FAIL as observed.

For T08, replace the placeholders and run from the client instance:

```sh
printf 'POST / HTTP/1.1\r\nHost: <SERVER_IP>:<PORT>\r\n\r\n' | nc <SERVER_IP> <PORT>
printf 'GARBAGE\r\n\r\n' | nc <SERVER_IP> <PORT>
printf 'GET / HTTP/1.1\r\nHost: <SERVER_IP>:<PORT>\r\n\r\n' | nc <SERVER_IP> <PORT>
```
