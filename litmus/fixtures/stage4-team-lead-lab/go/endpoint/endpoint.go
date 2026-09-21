package endpoint

import "net/url"

func Validate(raw string, allowedHosts []string) (*url.URL, error) {
	return url.Parse(raw)
}
