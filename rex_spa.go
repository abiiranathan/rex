package rex

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"
)

// SPAOptions for customizing the cache control and index file.
type SPAOption func(*spaHandler)

type spaHandler struct {
	indexContent     []byte
	indexModTime     time.Time
	cacheControl     string
	skipFunc         func(r *http.Request) bool
	responseModifier http.HandlerFunc
	fileServer       http.Handler
}

// WithCacheControl sets the Cache-Control header for the index file.
func WithCacheControl(cacheControl string) SPAOption {
	return func(h *spaHandler) {
		h.cacheControl = cacheControl
	}
}

// WithSkipFunc sets a function to skip the SPA handler for certain requests.
func WithSkipFunc(skipFunc func(r *http.Request) bool) SPAOption {
	return func(h *spaHandler) {
		h.skipFunc = skipFunc
	}
}

// WithResponseModifier sets a function to modify the response before serving the index file.
func WithResponseModifier(responseModifier http.HandlerFunc) SPAOption {
	return func(h *spaHandler) {
		h.responseModifier = responseModifier
	}
}

// newSPAHandler creates and initializes a new SPA handler
func newSPAHandler(frontend http.FileSystem, index string, options ...SPAOption) (*spaHandler, error) {
	// Pre-load index file
	indexContent, modTime, err := loadIndexFile(frontend, index)
	if err != nil {
		return nil, fmt.Errorf("failed to load index file: %w", err)
	}

	spa := &spaHandler{
		indexContent:     indexContent,
		indexModTime:     modTime,
		cacheControl:     "",
		skipFunc:         nil,
		responseModifier: nil,
		fileServer:       http.FileServer(frontend),
	}

	// Apply options
	for _, opt := range options {
		opt(spa)
	}
	return spa, nil
}

// loadIndexFile reads the index file content and modification time
func loadIndexFile(fs http.FileSystem, indexPath string) ([]byte, time.Time, error) {
	f, err := fs.Open(indexPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return nil, time.Time{}, err
	}

	return content, stat.ModTime(), nil
}

// ServeHTTP handles the actual request
func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.skipFunc != nil && h.skipFunc(r) {
		http.NotFound(w, r)
		return
	}

	// If path has an extension, try serving as static file first
	if ext := filepath.Ext(r.URL.Path); ext != "" {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	// Serve index.html for SPA routes
	h.serveIndex(w, r)
}

// serveIndex serves the pre-loaded index file
func (h *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if h.cacheControl != "" {
		w.Header().Set("Cache-Control", h.cacheControl)
	}

	if h.responseModifier != nil {
		h.responseModifier(w, r)
	}

	http.ServeContent(w, r, "index.html", h.indexModTime, bytes.NewReader(h.indexContent))
}

// SPA serves a single page application (SPA) with the given index file.
// The index file is served for all routes that do not match a static file
// except for routes that are skipped by the skipFunc or have an extension.
//
// The frontend is served from the given http.FileSystem.
// You can use the CreateFileSystem function to create a new http.FileSystem from a fs.FS (e.g embed.FS).
// To customize the cache control and skip behavior, you can use the WithCacheControl and WithSkipFunc options.
func (r *Router) SPA(pattern string, index string, frontend http.FileSystem, options ...SPAOption) {
	handler, err := newSPAHandler(frontend, index, options...)
	if err != nil {
		panic(fmt.Errorf("failed to create SPA handler: %w", err))
	}

	// Apply global middleware
	wrappedHandler := r.chain(r.globalMiddlewares, WrapHandler(handler))
	r.mux.Handle(pattern, wrappedHandler)
}

// Creates a new http.FileSystem from the fs.FS (e.g embed.FS) with the root directory.
// This is useful for serving single page applications.
func CreateFileSystem(frontendFS fs.FS, root string) http.FileSystem {
	fsys, err := fs.Sub(frontendFS, root)
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}
