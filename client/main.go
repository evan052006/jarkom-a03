package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"a03"
)

// HttpResponse does not store the reason phrase, so the output restores the
// standard phrase for each A03 status.
var reasonPhrases = map[int]string{
	200: "OK",
	400: "Bad Request",
	404: "Not Found",
	405: "Method Not Allowed",
	406: "Not Acceptable",
}

func run(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("client", flag.ContinueOnError)
	flags.SetOutput(errOut)
	host := flags.String("host", "127.0.0.1", "server host or IP address")
	port := flags.Int("port", 0, "required server port (use your A01a port)")
	uri := flags.String("uri", "/", "origin-form request URI, including optional query")
	accept := flags.String("accept", "application/json", "Accept header value (quote lists containing spaces)")
	encoding := flags.String("encoding", "none", "none, identity, or gzip")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *host == "" || *port < 1 || *port > 65535 {
		return fmt.Errorf("provide -host and -port in 1..65535")
	}
	if !strings.HasPrefix(*uri, "/") || strings.ContainsAny(*uri, " \t\r\n") {
		return fmt.Errorf("-uri must be an origin-form URI without whitespace")
	}
	if strings.TrimSpace(*accept) == "" || strings.ContainsAny(*accept, "\r\n") {
		return fmt.Errorf("-accept must be a nonempty header value")
	}
	if *encoding != "none" && *encoding != "identity" && *encoding != "gzip" {
		return fmt.Errorf("-encoding must be none, identity, or gzip")
	}
	address := net.JoinHostPort(*host, strconv.Itoa(*port))
	headers := a03.Headers{"host": address, "accept": *accept, "connection": "close"}
	if *encoding == "gzip" {
		headers["accept-encoding"] = "gzip"
	}
	conn, err := net.DialTimeout("tcp", address, a03.IOTimeout)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(a03.IOTimeout)); err != nil {
		return fmt.Errorf("deadline: %w", err)
	}
	response, err := Fetch(HttpRequest{Method: "GET", URI: *uri, Version: a03.HTTPVersion, Headers: headers}, conn)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	status := strconv.Itoa(response.StatusCode)
	if reason, ok := reasonPhrases[response.StatusCode]; ok {
		status += " " + reason
	}
	fmt.Fprintf(out, "%s %s\n", response.Version, status)
	for _, name := range []string{"content-type", "content-encoding", "content-length", "connection"} {
		if value, ok := response.Headers[name]; ok {
			fmt.Fprintf(out, "%s: %s\n", name, value)
		}
	}
	fmt.Fprintf(out, "\n%s\n", response.Body)
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(1)
	}
}
