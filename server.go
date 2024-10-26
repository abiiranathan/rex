package rex

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"os/signal"
	"time"

	"golang.org/x/net/http2"
)

// Wrapper around the standard http.Server.
// Adds easy graceful shutdown, functional options for customizing the server, and HTTP/2 support.
type Server struct {
	*http.Server
}

// Option for configuring the server.
type ServerOption func(*Server)

// Create a new Server instance with HTTP/2 support.
func NewServer(addr string, handler http.Handler, options ...ServerOption) *Server {
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
	http2.ConfigureServer(server.Server, &http2.Server{})

	for _, option := range options {
		option(server)
	}
	return server
}

// Gracefully shuts down the server. The default timeout is 5 seconds
// to wait for pending connections.
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

	if err := s.Server.Shutdown(ctx); err != nil {
		panic(err)
	}
}

func WithReadTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.Server.ReadTimeout = d
	}
}

func WithWriteTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.Server.WriteTimeout = d
	}
}

func WithIdleTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.Server.IdleTimeout = d
	}
}

func WithTLSConfig(config *tls.Config) ServerOption {
	return func(s *Server) {
		// Ensure HTTP/2 support is maintained
		config.NextProtos = []string{"h2", "http/1.1"}
		s.Server.TLSConfig = config
	}
}

// New option to fine-tune HTTP/2 settings
func WithHTTP2Options(options http2.Server) ServerOption {
	return func(s *Server) {
		http2.ConfigureServer(s.Server, &options)
	}
}

// Open the certificate and key files and return a tls.Config
func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}
