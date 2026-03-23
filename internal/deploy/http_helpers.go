package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func newJSONRequest(method string, reqURL *url.URL, payload any) (*http.Request, error) {
	if reqURL == nil {
		return nil, fmt.Errorf("request URL cannot be nil")
	}

	var body io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(jsonBody)
	}

	return http.NewRequestWithContext(context.Background(), method, reqURL.String(), body)
}

func executeRequest(client *http.Client, req *http.Request, expectedHost string, prepare func(*http.Request)) ([]byte, error) {
	if err := validateTrustedURL(req.URL, expectedHost); err != nil {
		return nil, err
	}

	if prepare != nil {
		prepare(req)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, formatAPIError(resp.StatusCode, body)
	}

	return body, nil
}

func decodeJSONBody[T any](body []byte) (*T, error) {
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &result, nil
}

func executeJSONRequest[T any](client *http.Client, req *http.Request, expectedHost string, prepare func(*http.Request)) (*T, error) {
	body, err := executeRequest(client, req, expectedHost, prepare)
	if err != nil {
		return nil, err
	}
	return decodeJSONBody[T](body)
}

func formatAPIError(statusCode int, body []byte) error {
	details := strings.TrimSpace(string(body))
	if details == "" {
		return fmt.Errorf("API error %d", statusCode)
	}
	return fmt.Errorf("API error %d: %s", statusCode, details)
}
