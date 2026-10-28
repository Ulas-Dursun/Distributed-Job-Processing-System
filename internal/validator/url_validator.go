package validator

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// URLFetchPayload is the expected JSON input payload for URL_FETCH jobs.
type URLFetchPayload struct {
	URL string `json:"url"`
}

// ValidateTargetURL verifies URL format, scheme, and guards against SSRF (RFC 1918 / loopback / metadata).
func ValidateTargetURL(rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return errors.New("unsupported protocol (only http and https allowed)")
	}

	host := parsed.Hostname()
	if host == "" {
		return errors.New("empty host in URL")
	}

	// Allow bypassing SSRF filter in automated local tests
	if os.Getenv("ALLOW_LOCAL_FETCH") == "true" {
		return nil
	}

	// Resolve host and check for private / loopback IP addresses (SSRF defense)
	ips, err := net.LookupIP(host)
	if err != nil {
		// If hostname is already an IP address
		ip := net.ParseIP(host)
		if ip != nil {
			ips = []net.IP{ip}
		} else {
			return fmt.Errorf("failed to resolve host: %w", err)
		}
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("target resolves to disallowed or private IP (%s)", ip.String())
		}
	}

	return nil
}
