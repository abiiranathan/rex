package rex

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"golang.org/x/net/http2"
)

// Server wraps http.Server with graceful shutdown helpers and option-based configuration.
type Server struct {
	*http.Server

	// h2 holds custom HTTP/2 settings supplied via WithHTTP2Options.
	// HTTP/2 is configured exactly once after all options have been applied.
	h2 *http2.Server
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// NewServer creates a Server with HTTP/2 support.
//
// All options are applied before HTTP/2 is configured, so custom HTTP/2
// settings and TLS configurations take effect without registering the
// HTTP/2 handler more than once.
func NewServer(addr string, handler http.Handler, options ...ServerOption) (*Server, error) {
	server := &Server{
		Server: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  15 * time.Second,
			TLSConfig: &tls.Config{
				NextProtos: []string{"h2", "http/1.1"},
			},
		},
	}

	for _, option := range options {
		option(server)
	}

	// Explicitly enable HTTP/2 (exactly once).
	if server.h2 == nil {
		server.h2 = &http2.Server{}
	}
	if err := http2.ConfigureServer(server.Server, server.h2); err != nil {
		return nil, err
	}

	return server, nil
}

// Shutdown gracefully shuts down the server.
// The default timeout is 5 seconds to wait for pending connections.
func (s *Server) Shutdown(ctx context.Context, timeout ...time.Duration) {
	var waitTimeout = 5 * time.Second
	if len(timeout) > 0 {
		waitTimeout = timeout[0]
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	if err := s.Server.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not gracefully shutdown the server: %v\n", err)
	}
}

// WithReadTimeout sets the server read timeout.
func WithReadTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.ReadTimeout = d
	}
}

// WithWriteTimeout sets the server write timeout.
func WithWriteTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.WriteTimeout = d
	}
}

// WithIdleTimeout sets the server idle timeout.
func WithIdleTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.IdleTimeout = d
	}
}

// WithTLSConfig sets the TLS configuration and preserves HTTP/2 support.
func WithTLSConfig(config *tls.Config) ServerOption {
	return func(s *Server) {
		// Ensure HTTP/2 support is maintained
		config.NextProtos = []string{"h2", "http/1.1"}
		s.TLSConfig = config
	}
}

// WithHTTP2Options configures HTTP/2 server settings.
// The settings are applied when NewServer finishes initializing the server;
// HTTP/2 is never registered more than once.
func WithHTTP2Options(http2ServerOptions http2.Server) ServerOption {
	return func(s *Server) {
		s.h2 = &http2ServerOptions
	}
}

// LoadTLSConfig loads a certificate pair and returns a TLS configuration.
func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}
