package desktop

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Single-instance activation: the running launcher listens on a loopback
// port with a random token, both written in instance.json (0600). A second
// launch on the same data directory sends {"token": ..., "action": "show"}
// there and exits; the first one brings its window to the front (or opens
// the panel in the browser for the headless UI). Whoever can read the data
// directory can read the database anyway, so the token only keeps other
// local users and web pages (no CORS-free way to speak this protocol) out.

// ActivationShow is the only action of the protocol.
const ActivationShow = "show"

const (
	activationTimeout = 2 * time.Second
	activationMaxLine = 512
)

// ErrActivationRefused: the running instance did not accept the message.
var ErrActivationRefused = errors.New("desktop: activation refused")

type activationMessage struct {
	Token  string `json:"token"`
	Action string `json:"action"`
}

// ActivationServer receives activation messages from later launches.
type ActivationServer struct {
	ln    net.Listener
	token string
	on    func()
	wg    sync.WaitGroup
}

// ListenActivation opens the activation listener on 127.0.0.1 with a new
// random token; onShow runs for each valid message (in its own goroutine).
func ListenActivation(onShow func()) (*ActivationServer, error) {
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("desktop: activation listener: %w", err)
	}
	s := &ActivationServer{ln: ln, token: hex.EncodeToString(tok), on: onShow}
	s.wg.Go(s.serve)
	return s, nil
}

// Addr is host:port of the listener.
func (s *ActivationServer) Addr() string { return s.ln.Addr().String() }

// Token is the shared secret written in instance.json.
func (s *ActivationServer) Token() string { return s.token }

// Close stops the listener and waits for the accept loop.
func (s *ActivationServer) Close() error {
	err := s.ln.Close()
	s.wg.Wait()
	return err
}

func (s *ActivationServer) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *ActivationServer) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(activationTimeout))
	if !isLoopbackAddr(c.RemoteAddr()) {
		return
	}
	line, err := bufio.NewReaderSize(c, activationMaxLine).ReadSlice('\n')
	if err != nil {
		return
	}
	var m activationMessage
	if json.Unmarshal(line, &m) != nil || m.Action != ActivationShow ||
		subtle.ConstantTimeCompare([]byte(m.Token), []byte(s.token)) != 1 {
		_, _ = c.Write([]byte("no\n"))
		return
	}
	_, _ = c.Write([]byte("ok\n"))
	if s.on != nil {
		go s.on()
	}
}

func isLoopbackAddr(a net.Addr) bool {
	t, ok := a.(*net.TCPAddr)
	return ok && t.IP.IsLoopback()
}

// SendActivation asks the instance listening on addr to show its window.
func SendActivation(ctx context.Context, addr, token string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("desktop: activation address %q is not loopback", addr)
	}
	allowForegroundActivation()
	d := net.Dialer{Timeout: activationTimeout}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(activationTimeout))
	b, _ := json.Marshal(activationMessage{Token: token, Action: ActivationShow})
	if _, err := c.Write(append(b, '\n')); err != nil {
		return err
	}
	reply, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(reply) != "ok" {
		return ErrActivationRefused
	}
	return nil
}
