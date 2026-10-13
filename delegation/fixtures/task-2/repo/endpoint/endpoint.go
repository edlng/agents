package endpoint

import (
	"errors"
	"net"
	"net/url"
	"strings"
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
	if net.ParseIP(host) != nil {
		return nil, errors.New("endpoint: IP literal hosts are not allowed")
	}
	for _, allowed := range allowedHosts {
		// Allow the listed host and anything under it.
		if strings.HasSuffix(host, strings.ToLower(allowed)) {
			return u, nil
		}
	}
	return nil, errors.New("endpoint: host not allowed")
}
