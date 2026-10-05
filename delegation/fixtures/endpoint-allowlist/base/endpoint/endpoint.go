package endpoint

import "net/url"

// Validate parses an outbound webhook URL.
func Validate(raw string, allowedHosts []string) (*url.URL, error) {
	return url.Parse(raw)
}
