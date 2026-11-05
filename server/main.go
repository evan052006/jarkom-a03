package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"a03"
)

func validNPM(npm string) bool {
	if len(npm) != 10 {
		return false
	}
	for _, digit := range npm {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// serve handles one connection. recover keeps one faulty request from stopping
// later connections; handlers must still not panic.
func serve(conn net.Conn, logger *log.Logger) {
	defer conn.Close()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Printf("%s: panic: %v", conn.RemoteAddr(), recovered)
		}
	}()
	if err := conn.SetDeadline(time.Now().Add(a03.IOTimeout)); err != nil {
		logger.Printf("deadline: %v", err)
		return
	}
	if err := HandleConnection(conn); err != nil {
		logger.Printf("%s: %v", conn.RemoteAddr(), err)
	}
}

func run(args []string, errOut io.Writer) error {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	flags.SetOutput(errOut)
	host := flags.String("host", "0.0.0.0", "listen interface (0.0.0.0 permits remote IPv4 clients)")
	port := flags.Int("port", 0, "required listen port (use your A01a port)")
	name := flags.String("name", "", "required student name")
	npm := flags.String("npm", "", "required 10-digit student NPM")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *port < 1 || *port > 65535 {
		return fmt.Errorf("provide -port in 1..65535")
	}
	if strings.TrimSpace(*name) == "" || !validNPM(*npm) {
		return fmt.Errorf("provide -name and a 10-digit decimal -npm")
	}
	ServerStudent = Student{Nama: *name, Npm: *npm}
	address := net.JoinHostPort(*host, strconv.Itoa(*port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	logger := log.New(errOut, "server: ", log.LstdFlags)
	// Log the configured address: Go reports a 0.0.0.0 listener as [::] because
	// it opens a dual-stack socket, which still accepts remote IPv4 clients.
	logger.Printf("listening on %s as %s (%s)", address, ServerStudent.Nama, ServerStudent.Npm)
	for {
		conn, err := listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("accept: %w", err)
		}
		if err != nil {
			// Transient failures (for example, too many open files) must not stop the server.
			logger.Printf("accept: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// Sequential handling is sufficient.
		serve(conn, logger)
	}
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
