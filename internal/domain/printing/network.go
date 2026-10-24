package printing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"syscall"
	"time"
)

// DriverNetwork is the printers.driver value of NetworkDriver.
const DriverNetwork = "network"

// Network driver timeouts (ADR 0021).
const (
	DefaultDialTimeout   = 3 * time.Second
	DefaultWriteTimeout  = 10 * time.Second
	DefaultStatusTimeout = 800 * time.Millisecond
)

// NetworkDriver sends raw ESC/POS bytes over TCP (port 9100, "JetDirect").
// Every connection is checked at dial time against AllowedIP, on the
// address actually dialled, so a host name that starts resolving to a
// public address is refused (no TCP proxy towards the Internet).
type NetworkDriver struct {
	DialTimeout   time.Duration // default 3 s
	WriteTimeout  time.Duration // default 10 s
	StatusTimeout time.Duration // wait for the DLE EOT answer, default 800 ms
}

var _ Driver = NetworkDriver{}

// Name implements Driver.
func (NetworkDriver) Name() string { return DriverNetwork }

func (d NetworkDriver) dial(ctx context.Context, t Target) (net.Conn, error) {
	dt := d.DialTimeout
	if dt <= 0 {
		dt = DefaultDialTimeout
	}
	dialer := net.Dialer{Timeout: dt, Control: guardPrivate}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(t.Host, strconv.Itoa(t.Port)))
	if err != nil {
		if errors.Is(err, ErrAddressNotAllowed) {
			return nil, ErrAddressNotAllowed
		}
		return nil, fmt.Errorf("printing: connect: %w", sanitizeNetErr(err))
	}
	return conn, nil
}

// guardPrivate refuses a connection to a disallowed address (Dialer.Control).
func guardPrivate(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrAddressNotAllowed
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !AllowedIP(ip) {
		return ErrAddressNotAllowed
	}
	return nil
}

// Send implements Driver: one connection, the whole payload, then close.
func (d NetworkDriver) Send(ctx context.Context, t Target, payload []byte) error {
	conn, err := d.dial(ctx, t)
	if err != nil {
		return err
	}
	defer conn.Close()
	wt := d.WriteTimeout
	if wt <= 0 {
		wt = DefaultWriteTimeout
	}
	deadline := time.Now().Add(wt)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("printing: write: %w", sanitizeNetErr(err))
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		// Half-close so the printer sees the end of the job, then close.
		_ = tc.CloseWrite()
	}
	return nil
}

// DLE EOT real-time status requests: n=2 offline cause, n=4 roll paper.
var statusRequest = []byte{0x10, 0x04, 0x02, 0x10, 0x04, 0x04}

// Check implements Driver: connects, asks DLE EOT 2 and 4 and reads the
// answers if the printer sends them (many clones do not: reachable anyway).
func (d NetworkDriver) Check(ctx context.Context, t Target) (Status, error) {
	start := time.Now()
	conn, err := d.dial(ctx, t)
	if err != nil {
		return Status{}, err
	}
	defer conn.Close()
	st := Status{Reachable: true, Latency: time.Since(start)}
	stt := d.StatusTimeout
	if stt <= 0 {
		stt = DefaultStatusTimeout
	}
	_ = conn.SetDeadline(time.Now().Add(stt))
	if _, err := conn.Write(statusRequest); err != nil {
		return st, nil
	}
	buf := make([]byte, 2)
	n, _ := io.ReadAtLeast(conn, buf, 2)
	if n >= 1 && validStatusByte(buf[0]) {
		st.CoverOpen = buf[0]&0x04 != 0
	}
	if n >= 2 && validStatusByte(buf[1]) {
		st.PaperLow = buf[1]&0x0c != 0 || buf[1]&0x60 != 0
	}
	return st, nil
}

// validStatusByte checks the fixed bits of a DLE EOT answer (0xx1xx10).
func validStatusByte(b byte) bool { return b&0x93 == 0x12 }

// sanitizeNetErr keeps the error class of a network error (refused,
// timeout, unreachable) without wrapping internals beyond the op text.
func sanitizeNetErr(err error) error {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errors.New("timeout")
	}
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return errors.New("connection refused")
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return errors.New("host unreachable")
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE):
		return errors.New("connection reset")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return errors.New("timeout")
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return errors.New("host name not found")
	}
	return err
}
