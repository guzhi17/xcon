// ------------------
// User: pei
// DateTime: 2020/10/27 16:21
// Description: 
// ------------------

package xcon

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"sync"
	"time"
)

type Session interface {
	OnData([]byte)error
}
type Handler interface {
	OnConn(*Conn)(Session, error)
	OnClose(Session, error)error
}

type PackageMode int
const (
	PmNone PackageMode = 0//iota //+ 1
	Pm16 PackageMode = 2
	Pm32 PackageMode = 4
)


type TLSConfig struct {
	CertFile, KeyFile string
}

func (c Config)ReadConfig() ReadConfig {
	return ReadConfig{
		PackageMaxLength: c.PackageMaxLength,
		PackageMode:      c.PackageMode,
		BufferLength:     c.BufferLength,
		ReadTimeout:      c.ReadTimeout,
	}
}



type Config struct {
	Handler          Handler

	ConnConfig

	Addr    string  // TCP address to listen on, ":" if empty

	*TLSConfig
	// BaseContext optionally specifies a function that returns
	// the base context for incoming requests on this server.
	// The provided Listener is the specific Listener that's
	// about to start accepting requests.
	// If BaseContext is nil, the default is context.Background().
	// If non-nil, it must return a non-nil context.
	BaseContext func(net.Listener) context.Context

	// ConnContext optionally specifies a function that modifies
	// the context used for a new connection c. The provided ctx
	// is derived from the base context and has a ServerContextKey
	// value.
	ConnContext func(ctx context.Context, c net.Conn) context.Context
}

// A Server defines parameters for running an HTTP server.
// The zero value for Server is a valid configuration.
type Server struct {
	Config

	// TLSConfig optionally provides a TLS configuration for use
	// by ServeTLS and ListenAndServeTLS. Note that this value is
	// cloned by ServeTLS and ListenAndServeTLS, so it's not
	// possible to modify the configuration with methods like
	// tls.Config.SetSessionTicketKeys. To use
	// SetSessionTicketKeys, use Server.Serve with a TLS Listener
	// instead.
	TLSConfig *tls.Config

	inShutdown        AtomInt32     // accessed atomically (non-zero means we're in Shutdown)

	mu         sync.Mutex
	listeners  map[*net.Listener]struct{}
	doneChan   chan struct{}
	onShutdown []func()
}

func CreateServer(c Config) *Server {
	if c.BufferLength < 64{
		c.BufferLength = 512
	}
	if c.PackageMaxLength < 1{
		c.PackageMaxLength = c.BufferLength * 10
	}
	return &Server{
		Config: c,
		listeners: map[*net.Listener]struct{}{},
	}
}

func (srv *Server) getDoneChan() <-chan struct{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.getDoneChanLocked()
}

func (srv *Server) getDoneChanLocked() chan struct{} {
	if srv.doneChan == nil {
		srv.doneChan = make(chan struct{})
	}
	return srv.doneChan
}

func (srv *Server) closeDoneChanLocked() {
	ch := srv.getDoneChanLocked()
	select {
	case <-ch:
		// Already closed. Don't close again.
	default:
		// Safe to close here. We're the only closer, guarded
		// by s.mu.
		close(ch)
	}
}

type contextKey struct {
	name string
}
func (k *contextKey) String() string { return "net/http context value " + k.name }

var (
	// ServerContextKey is a context key. It can be used in HTTP
	// handlers with Context.Value to access the server that
	// started the handler. The associated value will be of
	// type *Server.
	ServerContextKey = &contextKey{"tcp-server"}

	// LocalAddrContextKey is a context key. It can be used in
	// HTTP handlers with Context.Value to access the local
	// address the connection arrived on.
	// The associated value will be of type net.Addr.
	LocalAddrContextKey = &contextKey{"local-addr"}
)

func (srv *Server) shuttingDown() bool {
	// TODO: replace inShutdown with the existing atomicBool type;
	// see https://github.com/golang/go/issues/20239#issuecomment-381434582
	return srv.inShutdown.Load() != 0
}
var ErrServerClosed = errors.New("xconn: Server closed")
var ErrAbortHandler = errors.New("net/xconn: abort Handler")
var ErrPackageTooLarge = errors.New("net/xconn: package too large")
var ErrPackageModel = errors.New("net/xconn: package model failed")
var ErrConnClosed = errors.New("xconn closed")
var emptyStruct = struct{}{}
func (srv *Server) ListenAndServe() error {
	if srv.Config.TLSConfig!=nil{
		return srv.ListenAndServeTLS(srv.Config.TLSConfig.CertFile, srv.Config.TLSConfig.KeyFile)
	}else{
		return srv.ListenAndServeTcp()
	}
}
func (srv *Server) ListenAndServeTcp() error {
	if srv.shuttingDown() {
		return ErrServerClosed
	}
	addr := srv.Addr
	if addr == "" {
		addr = ":"
	}
	log.Println("Listen", addr)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	return srv.Serve(ln)
}
func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	if srv.shuttingDown() {
		return ErrServerClosed
	}
	addr := srv.Addr
	if addr == "" {
		addr = ":"
	}
	log.Println("Listen tls", addr)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	return srv.ServeTLS(ln, certFile, keyFile)
}

func (srv *Server) Serve(l net.Listener) error {
	var tempDelay time.Duration // how long to sleep on accept failure

	baseCtx := context.Background()
	if srv.BaseContext != nil {
		baseCtx = srv.BaseContext(l)
		if baseCtx == nil {
			panic("BaseContext returned a nil context")
		}
	}
	srv.listeners[&l] = emptyStruct
	ctx := context.WithValue(baseCtx, ServerContextKey, srv)
	for {
		rw, e := l.Accept()
		if e != nil {
			select {
			case <-srv.getDoneChan():
				return ErrServerClosed
			default:
			}
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				time.Sleep(tempDelay)
				continue
			}
			return e
		}
		connCtx := ctx
		if cc := srv.ConnContext; cc != nil {
			connCtx = cc(connCtx, rw)
			if connCtx == nil {
				panic("ConnContext returned nil")
			}
		}
		tempDelay = 0
		c := NewConn(rw, srv.ConnConfig)
		go c.serve(srv.Handler, connCtx)
	}
}

func cloneTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return &tls.Config{}
	}
	return cfg.Clone()
}

func (srv *Server) ServeTLS(l net.Listener, certFile, keyFile string) error {
	config := cloneTLSConfig(srv.TLSConfig)
	configHasCert := len(config.Certificates) > 0 || config.GetCertificate != nil
	if !configHasCert || certFile != "" || keyFile != "" {
		var err error
		config.Certificates = make([]tls.Certificate, 1)
		config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return err
		}
	}
	tlsListener := tls.NewListener(l, config)
	return srv.Serve(tlsListener)
}


func (srv *Server) closeListenersLocked() error {
	var err error
	for ln := range srv.listeners {
		if cerr := (*ln).Close(); cerr != nil && err == nil {
			err = cerr
		}
		delete(srv.listeners, ln)
	}
	return err
}

func (srv *Server) closeIdleConns() bool {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	quiescent := true
	return quiescent
}

var shutdownPollInterval = 500 * time.Millisecond
func (srv *Server) Shutdown(ctx context.Context) error {
	srv.inShutdown.Store(1)

	srv.mu.Lock()
	lnerr := srv.closeListenersLocked()
	srv.closeDoneChanLocked()
	for _, f := range srv.onShutdown {
		go f()
	}
	srv.mu.Unlock()

	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	for {
		if srv.closeIdleConns() {
			return lnerr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (srv *Server) RegisterOnShutdown(f func()) {
	srv.mu.Lock()
	srv.onShutdown = append(srv.onShutdown, f)
	srv.mu.Unlock()
}