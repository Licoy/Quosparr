package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

const (
	billingBaseURL      = "https://cli-chat-proxy.grok.com/v1"
	billingResetURL     = "https://grok.com/prod_mc_billing.ConsumerUiSvc/GetRemainingResets"
	billingTimeout      = 20 * time.Second
	billingMaxBodyBytes = 4 << 20
)

// BillingClient reads included credits for the account whose home holds
// auth.json. A nil HTTP client follows the current netx proxy.
type BillingClient struct {
	HTTP             *http.Client
	BaseURL          string
	ResetCreditsURL  string
	SubscriptionsURL string
	// Version returns the CLI version stamped on each request. Nil reads
	// `grok --version` once.
	Version func() string
	// Now stamps the snapshot. Nil means time.Now.
	Now func() time.Time
}

type billingPart struct {
	data []byte
	err  error
}

// Fetch implements BillingFunc. The JSON is the snapshot's MarshalJSON.
func (c *BillingClient) Fetch(ctx context.Context, homeDir string) (json.RawMessage, time.Time, error) {
	now := time.Now()
	if c != nil && c.Now != nil {
		now = c.Now()
	}
	snap, err := c.fetch(ctx, homeDir, now)
	if err != nil {
		return nil, time.Time{}, err
	}
	raw, err := snap.MarshalJSON()
	if err != nil {
		return nil, time.Time{}, err
	}
	return raw, now, nil
}

func (c *BillingClient) fetch(ctx context.Context, homeDir string, now time.Time) (BillingSnapshot, error) {
	token, err := accessTokenAt(homeDir)
	if err != nil {
		return BillingSnapshot{}, err
	}
	version := c.version()
	creditsCh := c.asyncGET(ctx, "billing", "format=credits", token, version)
	centsCh := c.asyncGET(ctx, "billing", "", token, version)
	settingsCh := c.asyncGET(ctx, "settings", "", token, version)
	resetCh := c.asyncReset(ctx, token, version)
	subscriptionCh := c.asyncSubscription(ctx, token, version)

	credits := <-creditsCh
	if stopped(credits.err) || errors.Is(credits.err, ErrAuthenticationRequired) {
		return BillingSnapshot{}, credits.err
	}
	cents := <-centsCh
	if errors.Is(cents.err, context.Canceled) || (credits.data == nil && errors.Is(cents.err, ErrAuthenticationRequired)) {
		return BillingSnapshot{}, cents.err
	}
	if cents.err != nil {
		cents = billingPart{}
	}
	snap, err := decodeEither(credits.data, cents.data, now)
	if err != nil {
		return BillingSnapshot{}, err
	}
	snap = mergeMoney(snap, cents.data)
	snap.Plan = rawBillingPlan(credits.data, cents.data)
	snap = mergePlan(snap, <-settingsCh, token)
	snap = mergeSubscription(snap, <-subscriptionCh)
	if err := ctx.Err(); err != nil {
		return BillingSnapshot{}, err
	}
	if reset := <-resetCh; reset.err == nil && len(reset.data) > 0 {
		if cards, decErr := DecodeResetCredits(reset.data, now); decErr == nil {
			snap = snap.MergingResetCredits(&cards)
		}
	}
	if err := ctx.Err(); err != nil {
		return BillingSnapshot{}, err
	}
	return snap, nil
}

func (c *BillingClient) version() string {
	if c != nil && c.Version != nil {
		return normalizedClientVersion(c.Version())
	}
	return defaultClientVersion()
}

func (c *BillingClient) base() string {
	if c != nil && c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return billingBaseURL
}

func (c *BillingClient) resetURL() string {
	if c != nil && c.ResetCreditsURL != "" {
		return c.ResetCreditsURL
	}
	return billingResetURL
}

func (c *BillingClient) asyncGET(ctx context.Context, path, rawQuery, token, version string) <-chan billingPart {
	ch := make(chan billingPart, 1)
	go func() {
		u := c.base() + "/" + path
		if rawQuery != "" {
			u += "?" + rawQuery
		}
		ch <- c.roundTrip(ctx, http.MethodGet, u, token, version, "application/json", "", nil)
	}()
	return ch
}

func (c *BillingClient) asyncReset(ctx context.Context, token, version string) <-chan billingPart {
	ch := make(chan billingPart, 1)
	go func() {
		ch <- c.roundTrip(ctx, http.MethodPost, c.resetURL(), token, version,
			"application/grpc-web+proto", "application/grpc-web+proto", ResetCreditsGRPCWebRequest(nil))
	}()
	return ch
}

func (c *BillingClient) roundTrip(ctx context.Context, method, rawURL, token, version, accept, contentType string, body []byte) billingPart {
	ctx, cancel := context.WithTimeout(ctx, billingTimeout)
	defer cancel()
	client, err := c.client(ctx)
	if err != nil {
		return billingPart{err: err}
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return billingPart{err: errors.New("billing request failed")}
	}
	applyIdentity(req.Header, token, version)
	req.Header.Set("Accept", accept)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-Grpc-Web", "1")
	}
	resp, err := client.Do(req)
	if err != nil {
		return billingPart{err: transportErr(err)}
	}
	defer resp.Body.Close()
	return readBilling(resp)
}

func (c *BillingClient) client(ctx context.Context) (*http.Client, error) {
	if c != nil && c.HTTP != nil {
		return c.HTTP, nil
	}
	return netx.ClientFor(ctx, netx.Standard)
}

func applyIdentity(h http.Header, token, version string) {
	v := normalizedClientVersion(version)
	h.Set("Authorization", "Bearer "+token)
	h.Set(tokenAuthHeader, tokenAuthValue)
	h.Set(clientVersionHeader, v)
	h.Set(clientIDHeader, clientIDValue)
	h.Set(clientModeHeader, clientModeValue)
	h.Set("User-Agent", userAgent(v))
}

func readBilling(resp *http.Response) billingPart {
	limited := io.LimitReader(resp.Body, billingMaxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return billingPart{err: transportErr(err)}
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return billingPart{err: ErrAuthenticationRequired}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return billingPart{err: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}
	if len(data) > billingMaxBodyBytes {
		return billingPart{err: &MalformedResponseError{Message: "billing parse failed"}}
	}
	return billingPart{data: data}
}

func transportErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return &AccountTimeoutError{Operation: "billing"}
	}
	return errors.New("billing request failed")
}

func stopped(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func accessTokenAt(homeDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(homeDir, "auth.json"))
	if err != nil {
		return "", ErrAuthenticationRequired
	}
	token, err := AccessTokenFromData(data)
	if err != nil || token == "" {
		return "", ErrAuthenticationRequired
	}
	return token, nil
}

func decodeEither(credits, cents []byte, now time.Time) (BillingSnapshot, error) {
	if snap, err := decodeBilling(credits, now); err == nil {
		return snap, nil
	}
	if snap, err := decodeBilling(cents, now); err == nil {
		return snap, nil
	}
	return BillingSnapshot{}, &MalformedResponseError{Message: "billing parse failed"}
}

func decodeBilling(data []byte, now time.Time) (BillingSnapshot, error) {
	if len(data) == 0 {
		return BillingSnapshot{}, ErrBillingUnknownStructure
	}
	return DecodeBillingResponse(data, now)
}

func mergeMoney(snap BillingSnapshot, cents []byte) BillingSnapshot {
	if len(cents) == 0 {
		return snap
	}
	limit, used, err := DecodeMoneyAllowance(cents)
	if err != nil {
		return snap
	}
	return snap.MergingMoneyAllowance(limit, used)
}
