package deploy

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecuteRequestRejectsUntrustedURL(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://api.digitalocean.com/v2/apps", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	_, err = executeRequest(http.DefaultClient, req, "api.digitalocean.com", nil)
	if err == nil {
		t.Fatal("expected untrusted URL error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected request scheme") {
		t.Fatalf("expected scheme validation error, got: %v", err)
	}
}

func TestExecuteRequestPropagatesErrorBody(t *testing.T) {
	reqURL, err := url.Parse("https://api.example.com/v1/apps")
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}

	req, err := newJSONRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("boom")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	_, err = executeRequest(client, req, "api.example.com", nil)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "API error 500: boom") {
		t.Fatalf("expected formatted API error with body, got: %v", err)
	}
}

func TestExecuteJSONRequestMalformedJSON(t *testing.T) {
	reqURL, err := url.Parse("https://api.example.com/v1/apps")
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}

	req, err := newJSONRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{bad json`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	type payload struct {
		ID string `json:"id"`
	}

	_, err = executeJSONRequest[payload](client, req, "api.example.com", nil)
	if err == nil {
		t.Fatal("expected JSON parsing error, got nil")
	}
	if !strings.Contains(err.Error(), "parsing response") {
		t.Fatalf("expected parsing response error, got: %v", err)
	}
}

func TestExecuteRequestTransportError(t *testing.T) {
	reqURL, err := url.Parse("https://api.example.com/v1/apps")
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}

	req, err := newJSONRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	wantErr := errors.New("network down")
	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, wantErr
		}),
	}

	_, err = executeRequest(client, req, "api.example.com", nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected transport error %v, got %v", wantErr, err)
	}
}
