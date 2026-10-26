package codexapi

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExchangeCodeKeepsHTTPStatusWithoutResponseBody(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500} {
		srv := newRecorder(t, func(recorded) (int, string) { return status, `{"error_description":"private-response-secret"}` })
		_, err := (OAuthLogin{HTTP: srv.Client(), TokenURL: srv.URL}).ExchangeCode(context.Background(), "fixture-code", LoginSession{}, fixedNow)
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != status || !NeedsReauth(err) {
			t.Fatal("exchange lost HTTP status or existing reauth contract")
		}
		if strings.Contains(err.Error(), "private-response-secret") {
			t.Fatal("exchange exposed response body")
		}
	}
}
