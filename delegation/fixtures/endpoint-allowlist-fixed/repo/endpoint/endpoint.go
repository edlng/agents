package endpoint

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode"
)

// Validate parses an outbound webhook URL and checks it against the host
// allowlist before the service sends any request to it.
func Validate(raw string, allowedHosts []string) (*url.URL, error) {
	if len(allowedHosts) == 0 {
		return nil, errors.New("endpoint: empty allowlist")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		return nil, errors.New("endpoint: https required")
	}
	if u.User != nil {
		return nil, errors.New("endpoint: credentials in URL")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil, errors.New("endpoint: missing host")
	}
	for _, r := range u.Hostname() {
		// Non-ASCII hosts must arrive in punycode. Check before lowercasing:
		// Unicode case folding maps look-alikes such as the Kelvin sign
		// (U+212A) to ASCII, so they would match an ASCII allowlist entry.
		if r > unicode.MaxASCII {
			return nil, errors.New("endpoint: non-ASCII host")
		}
	}
	if net.ParseIP(host) != nil {
		return nil, errors.New("endpoint: IP literal hosts are not allowed")
	}
	for _, allowed := range allowedHosts {
		if allowed != "" && host == strings.ToLower(allowed) {
			return u, nil
		}
	}
	return nil, errors.New("endpoint: host not allowed")
}
