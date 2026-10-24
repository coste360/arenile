// Package printertest is an in-process fake network ESC/POS printer for
// tests: a TCP listener on 127.0.0.1 that records every connection's bytes
// as one job and can answer DLE EOT status requests.
package printertest

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Printer is a fake network printer.
type Printer struct {
	ln   net.Listener
	mu   sync.Mutex
	jobs [][]byte
	got  chan struct{}
	// Status is the byte answered to every DLE EOT request (0 = silent).
	status byte
	wg     sync.WaitGroup
}

// Option configures Start.
type Option func(*Printer)

// WithStatus makes the printer answer DLE EOT n with b (e.g. 0x12 = ok,
// 0x16 = cover open on n=2, 0x1e = paper near end on n=4).
func WithStatus(b byte) Option { return func(p *Printer) { p.status = b } }

// Start listens on 127.0.0.1 and serves until the test ends.
func Start(t testing.TB, opts ...Option) *Printer {
	t.Helper()
	p, err := StartOn(t, 0, opts...)
	if err != nil {
		t.Fatalf("printertest: listen: %v", err)
	}
	return p
}

// StartOn is Start on a given port (0 = any), for a printer that comes
// back on the address it had.
func StartOn(t testing.TB, port int, opts ...Option) (*Printer, error) {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	p := &Printer{ln: ln, got: make(chan struct{}, 1024)}
	for _, o := range opts {
		o(p)
	}
	p.wg.Add(1)
	go p.serve()
	t.Cleanup(p.Close)
	return p, nil
}

// Host is the listening address (127.0.0.1).
func (p *Printer) Host() string { return "127.0.0.1" }

// Port is the listening port.
func (p *Printer) Port() int {
	addr, ok := p.ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0
	}
	return addr.Port
}

// Addr is host:port.
func (p *Printer) Addr() string { return net.JoinHostPort(p.Host(), strconv.Itoa(p.Port())) }

// Close stops the listener (the port then refuses connections).
func (p *Printer) Close() {
	_ = p.ln.Close()
	p.wg.Wait()
}

func (p *Printer) serve() {
	defer p.wg.Done()
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		p.wg.Add(1)
		go p.handle(c)
	}
}

func (p *Printer) handle(c net.Conn) {
	defer p.wg.Done()
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	var buf bytes.Buffer
	chunk := make([]byte, 4096)
	for {
		n, err := c.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
			if p.status != 0 {
				// Answer each complete DLE EOT n seen in this chunk.
				for i := 0; i+1 < n; i++ {
					if chunk[i] == 0x10 && chunk[i+1] == 0x04 {
						_, _ = c.Write([]byte{p.status})
					}
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && buf.Len() == 0 {
				return
			}
			break
		}
	}
	data := buf.Bytes()
	if isStatusOnly(data) {
		return // a Check, not a job
	}
	p.mu.Lock()
	p.jobs = append(p.jobs, bytes.Clone(data))
	p.mu.Unlock()
	p.got <- struct{}{}
}

// isStatusOnly reports whether data only holds DLE EOT requests (or nothing).
func isStatusOnly(data []byte) bool {
	for len(data) >= 3 && data[0] == 0x10 && data[1] == 0x04 {
		data = data[3:]
	}
	return len(data) == 0
}

// Jobs returns a copy of the payloads received so far, in order.
func (p *Printer) Jobs() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.jobs))
	for i, j := range p.jobs {
		out[i] = bytes.Clone(j)
	}
	return out
}

// WaitJobs waits until at least n jobs arrived or the timeout expires and
// returns the jobs received.
func (p *Printer) WaitJobs(n int, timeout time.Duration) [][]byte {
	deadline := time.After(timeout)
	for {
		if js := p.Jobs(); len(js) >= n {
			return js
		}
		select {
		case <-p.got:
		case <-deadline:
			return p.Jobs()
		}
	}
}
