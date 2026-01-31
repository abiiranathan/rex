package security

import (
	"fmt"

	"github.com/abiiranathan/rex"
)

// Config defines the configuration for Security middleware.
type Config struct {
	// XSSProtection sets the X-XSS-Protection header value.
	// This header enables the Cross-Site Scripting (XSS) filter built into most recent web browsers.
	// Recommended: "1; mode=block".
	XSSProtection string

	// ContentTypeNosniff sets the X-Content-Type-Options header value.
	// It prevents the browser from MIME-sniffing a response away from the declared content-type.
	// Recommended: "nosniff".
	ContentTypeNosniff string

	// XFrameOptions sets the X-Frame-Options header value.
	// It indicates whether or not a browser should be allowed to render a page in a <frame>, <iframe>, <embed> or <object>.
	// Recommended: "SAMEORIGIN" (allows iframe only from same domain) or "DENY" (blocks all iframes).
	XFrameOptions string

	// HSTSMaxAge sets the Strict-Transport-Security header max-age value.
	// This tells the browser to remember to only access the site using HTTPS for the specified time (in seconds).
	// Default: 0 (disabled). Recommended: 31536000 (1 year).
	HSTSMaxAge int

	// HSTSExcludeSubdomains if true, excludes subdomains from HSTS policy.
	// Default: false (subdomains are included if HSTS is enabled).
	HSTSExcludeSubdomains bool

	// HSTSPreload if true, adds "preload" directive to HSTS header.
	// This allows the domain to be submitted to the browser's HSTS Preload list.
	HSTSPreload bool

	// ContentSecurityPolicy (CSP) sets the Content-Security-Policy header value.
	// CSP is a security layer that helps detect and mitigate certain types of attacks, including XSS and data injection.
	// Example: "default-src 'self'; script-src 'self' https://trusted.cdn.com"
	// Default: "" (disabled).
	ContentSecurityPolicy string

	// ReferrerPolicy sets the Referrer-Policy header value.
	// It controls how much referrer information (the URL you came from) is sent with requests.
	// Example: "no-referrer" or "strict-origin-when-cross-origin".
	// Default: "" (disabled).
	ReferrerPolicy string
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		XSSProtection:      "1; mode=block",
		ContentTypeNosniff: "nosniff",
		XFrameOptions:      "SAMEORIGIN",
	}
}

// New creates a new Security middleware with the default configuration.
func New() rex.Middleware {
	return WithConfig(DefaultConfig())
}

// WithConfig creates a new Security middleware with the given configuration.
// It sets various HTTP security headers to protect against common web attacks.
//
// Understanding the Options:
//
// 1. XSSProtection (X-XSS-Protection):
// Cross-Site Scripting (XSS) is an attack where malicious scripts are injected into your website.
// This header tells browsers to stop pages from loading when they detect reflected XSS attacks.
// Recommended: "1; mode=block".
//
// 2. ContentTypeNosniff (X-Content-Type-Options):
// Browsers sometimes try to "guess" the file type (MIME sniffing) even if you specify one.
// Attackers can exploit this to execute malicious code (e.g., treating a text file as JavaScript).
// Setting this to "nosniff" forces the browser to strictly follow the Content-Type header you sent.
// Recommended: "nosniff".
//
// 3. XFrameOptions (X-Frame-Options):
// Clickjacking is an attack where your site is hidden inside an iframe on a malicious site,
// tricking users into clicking buttons they didn't intend to.
// This header controls whether your site can be embedded in an iframe.
// Options: "DENY" (never allow), "SAMEORIGIN" (allow only on your own site).
// Recommended: "SAMEORIGIN" or "DENY".
//
// 4. HSTS (Strict-Transport-Security):
// HTTP Strict Transport Security tells browsers to ALWAYS use HTTPS for your site,
// even if a user types "http://". This prevents "man-in-the-middle" attacks.
// Note: This requires your site to actually support HTTPS.
// HSTSMaxAge: How long (in seconds) the browser should remember this rule. Recommended: 31536000 (1 year).
// HSTSExcludeSubdomains: If true, the rule won't apply to subdomains (e.g., blog.yoursite.com).
// HSTSPreload: Allows your site to be added to the global browser HSTS preload list.
//
// 5. ContentSecurityPolicy (Content-Security-Policy):
// CSP is a powerful layer of security that helps detect and mitigate certain types of attacks,
// including Cross-Site Scripting (XSS) and data injection attacks.
// It allows you to restrict the resources (JavaScript, CSS, Images, etc.) that the browser is allowed to load.
// Example: "default-src 'self'" allows resources only from your own domain.
//
// 6. ReferrerPolicy (Referrer-Policy):
// Controls how much information is included in the 'Referer' header when navigating away from your site.
// This helps protect user privacy.
// Example: "no-referrer" sends no referrer information. "same-origin" sends it only for requests to the same site.
func WithConfig(config Config) rex.Middleware {
	if config.XSSProtection == "" {
		config.XSSProtection = "1; mode=block"
	}
	if config.ContentTypeNosniff == "" {
		config.ContentTypeNosniff = "nosniff"
	}
	if config.XFrameOptions == "" {
		config.XFrameOptions = "SAMEORIGIN"
	}

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			if config.XSSProtection != "" {
				c.SetHeader("X-XSS-Protection", config.XSSProtection)
			}
			if config.ContentTypeNosniff != "" {
				c.SetHeader("X-Content-Type-Options", config.ContentTypeNosniff)
			}
			if config.XFrameOptions != "" {
				c.SetHeader("X-Frame-Options", config.XFrameOptions)
			}
			if config.HSTSMaxAge > 0 {
				hsts := fmt.Sprintf("max-age=%d", config.HSTSMaxAge)
				if !config.HSTSExcludeSubdomains {
					hsts += "; includeSubDomains"
				}
				if config.HSTSPreload {
					hsts += "; preload"
				}
				c.SetHeader("Strict-Transport-Security", hsts)
			}
			if config.ContentSecurityPolicy != "" {
				c.SetHeader("Content-Security-Policy", config.ContentSecurityPolicy)
			}
			if config.ReferrerPolicy != "" {
				c.SetHeader("Referrer-Policy", config.ReferrerPolicy)
			}

			return next(c)
		}
	}
}
