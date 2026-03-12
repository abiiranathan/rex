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
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// NewServer creates a Server with HTTP/2 support.
func NewServer(addr string, handler http.Handler, options ...ServerOption) (*Server, error) {
	server := &Server{
		&http.Server{
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

	// Explicitly enable HTTP/2
	if err := http2.ConfigureServer(server.Server, &http2.Server{}); err != nil {
		return nil, err
	}

	for _, option := range options {
		option(server)
	}
	return server, nil
}

// Shutdown gracefully shuts down the server.
// The default timeout is 5 seconds to wait for pending connections.
func (s *Server) Shutdown(timeout ...time.Duration) {
	var t time.Duration
	if len(timeout) > 0 {
		t = timeout[0]
	} else {
		t = 5 * time.Second
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), t)
	defer cancel()

	if err := s.Server.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not gracefully shutdown the server: %v\n", err)
	}
}

// WithReadTimeout sets the server read timeout.
func WithReadTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.Server.ReadTimeout = d
	}
}

// WithWriteTimeout sets the server write timeout.
func WithWriteTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.Server.WriteTimeout = d
	}
}

// WithIdleTimeout sets the server idle timeout.
func WithIdleTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.Server.IdleTimeout = d
	}
}

// WithTLSConfig sets the TLS configuration and preserves HTTP/2 support.
func WithTLSConfig(config *tls.Config) ServerOption {
	return func(s *Server) {
		// Ensure HTTP/2 support is maintained
		config.NextProtos = []string{"h2", "http/1.1"}
		s.Server.TLSConfig = config
	}
}

// WithHTTP2Options configures HTTP/2 server settings.
func WithHTTP2Options(http2ServerOptions http2.Server) ServerOption {
	return func(s *Server) {
		if err := http2.ConfigureServer(s.Server, &http2ServerOptions); err != nil {
			log.Println(err)
		}
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
