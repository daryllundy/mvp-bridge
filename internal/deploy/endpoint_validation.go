package deploy

import (
	"fmt"
	"net/url"
)

func mustParseURL(rawURL string) *url.URL {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		panic(fmt.Sprintf("invalid URL %q: %v", rawURL, err))
	}

	return parsed
}

func validateTrustedURL(reqURL *url.URL, expectedHost string) error {
	if reqURL == nil {
		return fmt.Errorf("request URL cannot be nil")
	}

	if reqURL.Scheme != "https" {
		return fmt.Errorf("unexpected request scheme: %s", reqURL.Scheme)
	}

	if reqURL.Host != expectedHost {
		return fmt.Errorf("unexpected request host: %s", reqURL.Host)
	}

	return nil
}

func mustJoinURLPath(baseURL *url.URL, elems ...string) *url.URL {
	return baseURL.JoinPath(elems...)
}
