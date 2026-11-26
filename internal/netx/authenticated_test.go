package netx

import (
	"net/http"
	"net/url"
	"testing"
)

func TestAuthenticatedRedirectsStayOnOrigin(t *testing.T) {
	original, err := http.NewRequest(http.MethodPost, "https://login.example.test/token", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, target string
		allowed      bool
	}{
		{"same origin", "https://login.example.test/new", true},
		{"explicit default port", "https://LOGIN.example.test:443/new", true},
		{"different host", "https://other.example.test/new", false},
		{"subdomain", "https://other.login.example.test/new", false},
		{"different port", "https://login.example.test:8443/new", false},
		{"downgrade", "http://login.example.test/new", false},
		{"embedded userinfo", "https://name@login.example.test/new", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			to, err := url.Parse(tt.target)
			if err != nil {
				t.Fatal(err)
			}
			if got := sameHTTPSOrigin(original.URL, to); got != tt.allowed {
				t.Fatalf("sameHTTPSOrigin() = %t, want %t", got, tt.allowed)
			}
		})
	}
}

func TestAuthenticatedRequestRejectsHTTP(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://relay.example.invalid/model", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := DoAuthenticated(http.DefaultClient, req); err != ErrHTTPSRequired {
		t.Fatalf("DoAuthenticated() error = %v, want ErrHTTPSRequired", err)
	}
}

func TestDoAuthenticatedHonorsRedirectPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, location string
		status         int
	}{
		{"same origin", "/new", http.StatusOK},
		{"other origin", "https://other.example.test/new", http.StatusTemporaryRedirect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: authenticatedTestTransport(tc.location)}
			req, err := http.NewRequest(http.MethodGet, "https://login.example.test/", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := DoAuthenticated(client, req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.status)
			}
		})
	}
}

type authenticatedTestTransport string

func (location authenticatedTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/" {
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": {string(location)}},
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}
