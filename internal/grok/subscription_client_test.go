package grok

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBillingFetchSubscriptionMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, user, subscriptions, wantPlan, wantError string
		status                                         int
	}{
		{"commerce", `{"userId":"account-a","subscriptionTier":"SuperGrokPro"}`, `{"subscriptions":[{"tier":"SUBSCRIPTION_TIER_SUPER_GROK_PRO","status":"SUBSCRIPTION_STATUS_ACTIVE","billingSystem":"BILLING_SYSTEM_COMMERCE","xaiUserId":"account-a"}]}`, "X · Ultra", "", 200},
		{"metadata denied", `{"userId":"account-a","subscriptionTier":"SuperGrokPro"}`, `secret-response-body`, "SuperGrok Heavy", "subscriptions: HTTP 403", 403},
		{"metadata server error", `{"userId":"account-a"}`, `secret-response-body`, "SuperGrok Heavy", "subscriptions: HTTP 503", 503},
		{"invalid json", `{"userId":"account-a"}`, `{"subscriptions":`, "SuperGrok Heavy", "subscriptions: invalid response", 200},
		{"missing user id", `{}`, `{"subscriptions":[]}`, "SuperGrok Heavy", "user: invalid response", 200},
		{"missing subscriptions", `{"userId":"account-a"}`, `{}`, "SuperGrok Heavy", "subscriptions: invalid response", 200},
		{"identity mismatch", `{"userId":"account-a","subscriptionTier":"SuperGrokPro"}`, `{"subscriptions":[{"tier":"SUBSCRIPTION_TIER_SUPER_GROK_PRO","status":"SUBSCRIPTION_STATUS_ACTIVE","billingSystem":"BILLING_SYSTEM_COMMERCE","xaiUserId":"different-account"}]}`, "SuperGrok Heavy", "subscriptions: identity mismatch", 200},
		{"empty subscriptions", `{"userId":"account-a"}`, `{"subscriptions":[]}`, "SuperGrok Heavy", "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := syntheticTierToken(1)
			var subscriptionRequests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertIdentity(t, r, token, "9.9.9")
				if (r.URL.Path == "/v1/user" || r.URL.Path == "/subscriptions") && r.Header.Get("Cookie") != "" {
					t.Error("subscription request sent cookie")
				}
				switch r.URL.Path {
				case "/v1/billing":
					writeJSON(w, `{"config":{"creditUsagePercent":12,"monthlyLimit":15000}}`)
				case "/v1/settings":
					writeJSON(w, `{"subscription_tier_display":"SuperGrok Heavy"}`)
				case "/v1/user":
					if r.URL.Query().Get("include") != "subscription" {
						t.Error("missing user include")
					}
					writeJSON(w, tc.user)
				case "/subscriptions":
					subscriptionRequests.Add(1)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.subscriptions))
				case "/reset":
					writeJSON(w, `{"tokens":[]}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			client := srv.Client()
			client.Jar, _ = cookiejar.New(nil)
			serverURL, _ := url.Parse(srv.URL)
			client.Jar.SetCookies(serverURL, []*http.Cookie{{Name: "browser", Value: "private"}})
			home := t.TempDir()
			writeAuth(t, home, token)
			billing := &BillingClient{HTTP: client, BaseURL: srv.URL + "/v1", ResetCreditsURL: srv.URL + "/reset", SubscriptionsURL: srv.URL + "/subscriptions", Version: func() string { return "9.9.9" }}
			raw, _, err := billing.Fetch(context.Background(), home)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := ParseBillingSnapshot(raw)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Plan == nil || *snapshot.Plan != tc.wantPlan || snapshot.SubscriptionError != tc.wantError || snapshot.IncludedUsagePercent != 12 {
				t.Fatalf("plan %v error %q usage %v", snapshot.Plan, snapshot.SubscriptionError, snapshot.IncludedUsagePercent)
			}
			if strings.Contains(string(raw), "secret-response-body") || strings.Contains(string(raw), "account-a") {
				t.Fatal("metadata identifiers/body escaped into snapshot")
			}
			if tc.name == "missing user id" && subscriptionRequests.Load() != 0 {
				t.Fatal("missing identity still requested subscriptions")
			}
		})
	}
}

func TestSubscriptionRequestsRejectRedirectsAndReportCancellation(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer source.Close()
	client := &BillingClient{HTTP: source.Client()}
	_, err := client.subscriptionGET(context.Background(), source.URL, "test-token", "9.9.9")
	if err == nil || err.Error() != "HTTP 302" || redirected.Load() != 0 {
		t.Fatalf("redirect err %v count %d", err, redirected.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.subscriptionGET(ctx, source.URL, "test-token", "9.9.9")
	if err == nil || err.Error() != "canceled" {
		t.Fatalf("cancel = %v", err)
	}
	if got := subscriptionErrorText(errors.New(destination.URL + "?secret=secret-response-body")); got != "request failed" {
		t.Fatalf("unsafe error %q", got)
	}
}

func TestBillingCancellationDuringMetadataIsNotSuccess(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user":
			close(started)
			<-r.Context().Done()
		case "/reset":
			writeJSON(w, `{"tokens":[]}`)
		case "/v1/settings":
			writeJSON(w, `{"subscription_tier_display":"SuperGrok Heavy"}`)
		default:
			writeJSON(w, `{"config":{"creditUsagePercent":12,"monthlyLimit":15000}}`)
		}
	}))
	defer srv.Close()
	home := t.TempDir()
	writeAuth(t, home, syntheticTierToken(1))
	client := &BillingClient{HTTP: srv.Client(), BaseURL: srv.URL + "/v1", ResetCreditsURL: srv.URL + "/reset", SubscriptionsURL: srv.URL + "/subscriptions", Version: func() string { return "9.9.9" }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := client.Fetch(ctx, home); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("fetch err %v, want cancellation", err)
	}
}

func TestBillingCancellationWhileOnlyResetRemainsIsNotSuccess(t *testing.T) {
	completed := make(chan struct{}, 5)
	resetStarted := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/reset" {
			defer func() { completed <- struct{}{} }()
		}
		switch r.URL.Path {
		case "/reset":
			_, _ = io.Copy(io.Discard, r.Body)
			close(resetStarted)
			<-r.Context().Done()
		case "/v1/user":
			writeJSON(w, `{"userId":"a","subscriptionTier":"Free"}`)
		case "/subscriptions":
			writeJSON(w, `{"subscriptions":[]}`)
		case "/v1/settings":
			writeJSON(w, `{"subscription_tier_display":"Free"}`)
		default:
			writeJSON(w, `{"config":{"creditUsagePercent":0,"monthlyLimit":15000}}`)
		}
	}))
	defer srv.Close()
	home := t.TempDir()
	writeAuth(t, home, syntheticTierToken(0))
	client := &BillingClient{HTTP: srv.Client(), BaseURL: srv.URL + "/v1", ResetCreditsURL: srv.URL + "/reset", SubscriptionsURL: srv.URL + "/subscriptions", Version: func() string { return "9.9.9" }}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		snapshot bool
		err      error
	}
	done := make(chan result, 1)
	go func() {
		raw, when, err := client.Fetch(ctx, home)
		done <- result{len(raw) != 0 || !when.IsZero(), err}
	}()
	for range 5 {
		<-completed
	}
	<-resetStarted
	cancel()
	got := <-done
	if !errors.Is(got.err, context.Canceled) || got.snapshot {
		t.Fatalf("reset cancellation: err %v, snapshot present %v", got.err, got.snapshot)
	}
}
