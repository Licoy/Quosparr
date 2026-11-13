package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Cloud Code hosts.
const (
	DailyCloudCodeURL = "https://daily-cloudcode-pa.googleapis.com"
	ProdCloudCodeURL  = "https://cloudcode-pa.googleapis.com"
	// CloudCodeURLEnv overrides every Cloud Code host (development hook).
	CloudCodeURLEnv = "QUOSPARR_DEV_GEMINI_CLOUDCODE_URL"

	placeholderProject         = "aicode-consumers"
	reasonValidationRequired   = "VALIDATION_REQUIRED"
	reasonSubscriptionRequired = "SUBSCRIPTION_REQUIRED"
	defaultOnboardTier         = "LEGACY"
	quotaTimeout               = 20 * time.Second
	defaultPollInterval        = 500 * time.Millisecond
	defaultMaxPolls            = 20
	defaultModelRetries        = 2
	nodeClientUA               = "google-api-nodejs-client/10.3.0"
	nodeAPIClient              = "gl-node/22.21.1"
)

// QuotaClient reads the Cloud Code quota of an access token. Every field is
// optional; zero values use production behavior.
type QuotaClient struct {
	// HTTP nil follows the current netx proxy.
	HTTP *http.Client
	// BaseURL, when set, is used for every request (beats the environment).
	BaseURL string
	// DailyURL and ProdURL default to the Cloud Code hosts.
	DailyURL, ProdURL string
	// ClientVersion nil detects the installed Antigravity version.
	ClientVersion func() string
	// GOOS and GOARCH default to the running platform.
	GOOS, GOARCH string
	// PollInterval spaces onboarding polls and model retries (500ms).
	PollInterval time.Duration
	// MaxPolls bounds onboarding polls (20).
	MaxPolls int
	// ModelRetries is how often fetchAvailableModels is retried (2).
	ModelRetries int
	// Now stamps the snapshot; nil means time.Now.
	Now func() time.Time
}

type cloudTier struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	IsDefault        bool   `json:"isDefault"`
	UsesGcpTos       bool   `json:"usesGcpTos"`
	AvailableCredits []struct {
		CreditType                  string          `json:"creditType"`
		CreditAmount                json.RawMessage `json:"creditAmount"`
		MinimumCreditAmountForUsage json.RawMessage `json:"minimumCreditAmountForUsage"`
	} `json:"availableCredits"`
}

type loadCodeAssistResponse struct {
	Project      json.RawMessage `json:"cloudaicompanionProject"`
	CurrentTier  *cloudTier      `json:"currentTier"`
	PaidTier     *cloudTier      `json:"paidTier"`
	AllowedTiers []cloudTier     `json:"allowedTiers"`
}

type assist struct {
	project string
	tier    Tier
	credits []Credit
	gcpTos  bool
}

func (c *QuotaClient) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *QuotaClient) goos() string   { return firstNonEmpty(c.GOOS, runtime.GOOS) }
func (c *QuotaClient) goarch() string { return firstNonEmpty(c.GOARCH, runtime.GOARCH) }

func (c *QuotaClient) version() string {
	if c.ClientVersion != nil {
		return atLeastMinimum(c.ClientVersion())
	}
	return DefaultClientVersion()
}

// baseURL is the host for the account: BaseURL, else the environment, else
// production for GCP-terms accounts with a real project, else daily.
func (c *QuotaClient) baseURL(prod bool) string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	if env := trimSpace(os.Getenv(CloudCodeURLEnv)); env != "" {
		return strings.TrimRight(env, "/")
	}
	if prod {
		return strings.TrimRight(firstNonEmpty(c.ProdURL, ProdCloudCodeURL), "/")
	}
	return strings.TrimRight(firstNonEmpty(c.DailyURL, DailyCloudCodeURL), "/")
}

// Fetch reads one snapshot: loadCodeAssist (onboarding when no project is
// assigned), the quota summary and the model list. The model list fills in
// when the summary cannot be read; 401, 403 validation and 429 stop at once.
func (c *QuotaClient) Fetch(ctx context.Context, accessToken string) (QuotaSnapshot, error) {
	version := c.version()
	info, err := c.loadCodeAssist(ctx, accessToken, version)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	base := c.baseURL(info.gcpTos && info.project != "")
	body := map[string]any{}
	if info.project != "" {
		body["project"] = info.project
	}
	snap := QuotaSnapshot{Tier: info.tier, Project: info.project, Credits: info.credits, UpdatedAt: c.now()}
	buckets, summaryErr := c.quotaSummary(ctx, base, accessToken, version, body)
	if isFatal(summaryErr) {
		return QuotaSnapshot{}, summaryErr
	}
	models, modelsErr := c.availableModels(ctx, base, accessToken, version, body)
	if isFatal(modelsErr) && summaryErr != nil {
		return QuotaSnapshot{}, modelsErr
	}
	switch {
	case summaryErr == nil:
		snap.Source, snap.Buckets = SourceSummary, buckets
	case modelsErr == nil:
		snap.Source = SourceModels
	default:
		return QuotaSnapshot{}, summaryErr
	}
	if modelsErr == nil {
		snap.Models = models
	}
	return snap, nil
}

// isFatal: errors that the model list cannot work around.
func isFatal(err error) bool {
	var (
		validation *ValidationRequiredError
		limited    *RateLimitedError
	)
	return err != nil && (errors.Is(err, ErrUnauthorized) || errors.As(err, &validation) ||
		errors.As(err, &limited) || errors.Is(err, context.Canceled))
}

func (c *QuotaClient) metadata(version string) map[string]any {
	return map[string]any{
		"ideName": "antigravity", "ideType": "ANTIGRAVITY", "ideVersion": version, "pluginVersion": version,
		"platform": clientPlatform(c.goos(), c.goarch()), "updateChannel": "stable", "pluginType": "GEMINI",
	}
}

func (c *QuotaClient) assistHeaders(version string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", "antigravity/"+version+" "+clientOS(c.goos())+"/"+clientArch(c.goarch())+" "+nodeClientUA)
	h.Set("x-goog-api-client", nodeAPIClient)
	return h
}

func (c *QuotaClient) quotaHeaders(version string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", "antigravity/"+version+" (aidev_client; os_type="+clientOS(c.goos())+"; arch="+clientArch(c.goarch())+")")
	return h
}

func (c *QuotaClient) loadCodeAssist(ctx context.Context, token, version string) (assist, error) {
	base := c.baseURL(false)
	body := map[string]any{"metadata": c.metadata(version), "mode": "FULL_ELIGIBILITY_CHECK"}
	raw, err := c.call(ctx, http.MethodPost, base+"/v1internal:loadCodeAssist", "loadCodeAssist", token, c.assistHeaders(version), body)
	if err != nil {
		return assist{}, err
	}
	var resp loadCodeAssistResponse
	if json.Unmarshal(raw, &resp) != nil {
		return assist{}, ErrQuotaParse
	}
	info := assist{project: projectID(resp.Project), gcpTos: usesGcpTos(resp)}
	var paidID, paidName, currentID, currentName string
	if resp.PaidTier != nil {
		paidID, paidName = resp.PaidTier.ID, resp.PaidTier.Name
		info.credits = credits(resp.PaidTier)
	}
	if resp.CurrentTier != nil {
		currentID, currentName = resp.CurrentTier.ID, resp.CurrentTier.Name
	}
	info.tier = ResolveTier(paidID, paidName, currentID, currentName)
	if info.project == "" {
		project, err := c.onboard(ctx, base, token, version, onboardTier(resp.AllowedTiers))
		if isFatal(err) || errors.Is(err, context.DeadlineExceeded) {
			return assist{}, err
		}
		info.project = project
	}
	return info, nil
}

func usesGcpTos(resp loadCodeAssistResponse) bool {
	if resp.CurrentTier == nil {
		return false
	}
	if resp.CurrentTier.UsesGcpTos {
		return true
	}
	for _, t := range resp.AllowedTiers {
		if t.ID == resp.CurrentTier.ID && t.UsesGcpTos {
			return true
		}
	}
	return false
}

func onboardTier(allowed []cloudTier) string {
	for _, t := range allowed {
		if t.IsDefault && trimSpace(t.ID) != "" {
			return t.ID
		}
	}
	if len(allowed) > 0 && trimSpace(allowed[0].ID) != "" {
		return allowed[0].ID
	}
	return defaultOnboardTier
}

// projectID reads a project given as a string or as {"id": …}; the
// consumer placeholder counts as no project.
func projectID(raw json.RawMessage) string {
	id := rawString(raw)
	if id == "" {
		var o struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &o) == nil {
			id = o.ID
		}
	}
	id = trimSpace(id)
	if id == placeholderProject {
		return ""
	}
	return id
}

func credits(t *cloudTier) []Credit {
	var out []Credit
	for _, c := range t.AvailableCredits {
		out = append(out, Credit{
			Type: trimSpace(c.CreditType), Amount: flexibleNumber(c.CreditAmount),
			MinimumForUsage: flexibleNumber(c.MinimumCreditAmountForUsage),
		})
	}
	return out
}

// flexibleNumber reads a JSON number or a numeric string (int64 in proto JSON).
func flexibleNumber(raw json.RawMessage) float64 {
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f
	}
	if s := rawString(raw); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return 0
}

type operation struct {
	Name     string `json:"name"`
	Done     bool   `json:"done"`
	Response struct {
		Project json.RawMessage `json:"cloudaicompanionProject"`
	} `json:"response"`
}

// onboard runs onboardUser and polls the long-running operation every
// PollInterval, at most MaxPolls times. An empty project is not an error:
// personal accounts may stay without one.
func (c *QuotaClient) onboard(ctx context.Context, base, token, version, tierID string) (string, error) {
	headers := c.assistHeaders(version)
	body := map[string]any{"tierId": tierID, "metadata": c.metadata(version)}
	raw, err := c.call(ctx, http.MethodPost, base+"/v1internal:onboardUser", "onboardUser", token, headers, body)
	if err != nil {
		return "", err
	}
	interval := c.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	polls := c.MaxPolls
	if polls <= 0 {
		polls = defaultMaxPolls
	}
	for i := 0; ; i++ {
		var op operation
		if json.Unmarshal(raw, &op) != nil {
			return "", ErrQuotaParse
		}
		if op.Done || op.Name == "" {
			return projectID(op.Response.Project), nil
		}
		if i >= polls {
			return "", &TimeoutError{Operation: "onboardUser"}
		}
		if err := sleepContext(ctx, interval); err != nil {
			return "", err
		}
		raw, err = c.call(ctx, http.MethodGet, base+"/v1internal/"+strings.TrimLeft(op.Name, "/"), "onboardUser", token, headers, nil)
		if err != nil {
			return "", err
		}
	}
}

type summaryResponse struct {
	Groups []struct {
		DisplayName string `json:"displayName"`
		Name        string `json:"name"`
		Buckets     []struct {
			BucketID          string   `json:"bucketId"`
			DisplayName       string   `json:"displayName"`
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
		} `json:"buckets"`
	} `json:"groups"`
}

func (c *QuotaClient) quotaSummary(ctx context.Context, base, token, version string, body map[string]any) ([]QuotaBucket, error) {
	raw, err := c.call(ctx, http.MethodPost, base+"/v1internal:retrieveUserQuotaSummary", "retrieveUserQuotaSummary", token, c.quotaHeaders(version), body)
	if err != nil {
		return nil, err
	}
	return parseQuotaSummary(raw)
}

func parseQuotaSummary(raw []byte) ([]QuotaBucket, error) {
	var resp summaryResponse
	if json.Unmarshal(raw, &resp) != nil {
		return nil, ErrQuotaParse
	}
	var out []QuotaBucket
	for _, g := range resp.Groups {
		group := firstNonEmpty(g.DisplayName, g.Name)
		for _, b := range g.Buckets {
			id := trimSpace(b.BucketID)
			if id == "" {
				continue
			}
			fraction := 0.0
			if b.RemainingFraction != nil {
				fraction = *b.RemainingFraction
			}
			out = append(out, bucketFrom(id, group, trimSpace(b.DisplayName), fraction, parseResetTime(b.ResetTime)))
		}
	}
	return out, nil
}

type modelsResponse struct {
	Models map[string]struct {
		DisplayName string `json:"displayName"`
		Model       string `json:"model"`
		Recommended bool   `json:"recommended"`
		QuotaInfo   *struct {
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
		} `json:"quotaInfo"`
	} `json:"models"`
}

// availableModels calls fetchAvailableModels, retrying transient failures
// (transport errors and 5xx) up to ModelRetries times.
func (c *QuotaClient) availableModels(ctx context.Context, base, token, version string, body map[string]any) ([]ModelQuota, error) {
	retries := c.ModelRetries
	if retries <= 0 {
		retries = defaultModelRetries
	}
	interval := c.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, interval); err != nil {
				return nil, err
			}
		}
		raw, err := c.call(ctx, http.MethodPost, base+"/v1internal:fetchAvailableModels", "fetchAvailableModels", token, c.quotaHeaders(version), body)
		if err == nil {
			return parseAvailableModels(raw)
		}
		lastErr = err
		if !transient(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func transient(err error) bool {
	var status *StatusError
	if errors.As(err, &status) {
		return status.StatusCode >= 500
	}
	return !isFatal(err) && !errors.Is(err, ErrQuotaParse)
}

func parseAvailableModels(raw []byte) ([]ModelQuota, error) {
	var resp modelsResponse
	if json.Unmarshal(raw, &resp) != nil {
		return nil, ErrQuotaParse
	}
	out := make([]ModelQuota, 0, len(resp.Models))
	for id, m := range resp.Models {
		mq := ModelQuota{ID: id, DisplayName: trimSpace(m.DisplayName), Model: trimSpace(m.Model), Recommended: m.Recommended}
		if m.QuotaInfo != nil {
			fraction := 0.0
			if m.QuotaInfo.RemainingFraction != nil {
				fraction = clampFraction(*m.QuotaInfo.RemainingFraction)
			}
			mq.RemainingFraction = &fraction
			mq.ResetAt = parseResetTime(m.QuotaInfo.ResetTime)
		}
		out = append(out, mq)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func parseResetTime(s string) *time.Time {
	if s = trimSpace(s); s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	return &t
}

// call sends one Cloud Code request and maps the status.
func (c *QuotaClient) call(ctx context.Context, method, endpoint, name, token string, headers http.Header, body map[string]any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, quotaTimeout)
	defer cancel()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, reader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, nil)
	}
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header[k] = v
	}
	req.Header.Set("Accept", contentTypeJSON)
	req.Header.Set("Authorization", "Bearer "+token)
	if reader != nil {
		req.Header.Set("Content-Type", contentTypeJSON)
	}
	status, raw, respHeaders, err := doRequest(ctx, c.HTTP, req)
	if err != nil {
		return nil, transportError(ctx, name, err)
	}
	if status >= 200 && status <= 299 {
		return raw, nil
	}
	return nil, statusError(name, status, raw, respHeaders, c.now())
}

func statusError(name string, status int, body []byte, headers http.Header, now time.Time) error {
	switch status {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		wait, ok := retryAfter(headers.Get("Retry-After"), now)
		return &RateLimitedError{RetryAfter: wait, HasRetryAfter: ok}
	case http.StatusForbidden:
		if v, ok := validationFrom(body); ok {
			return &ValidationRequiredError{Reason: v.Reason, URL: v.URL}
		}
	}
	return &StatusError{Method: name, StatusCode: status}
}

// validationFrom reads a 403 body: a details entry whose reason is
// VALIDATION_REQUIRED or SUBSCRIPTION_REQUIRED, or that carries
// metadata.validation_url.
func validationFrom(body []byte) (Validation, bool) {
	var resp struct {
		Error struct {
			Details []struct {
				Reason   string            `json:"reason"`
				Metadata map[string]string `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &resp) != nil {
		return Validation{}, false
	}
	for _, d := range resp.Error.Details {
		link := safeValidationURL(d.Metadata["validation_url"])
		switch {
		case d.Reason == reasonValidationRequired || d.Reason == reasonSubscriptionRequired:
			return Validation{Reason: d.Reason, URL: link}, true
		case link != "":
			return Validation{Reason: firstNonEmpty(d.Reason, reasonValidationRequired), URL: link}, true
		}
	}
	return Validation{}, false
}

// safeValidationURL keeps an https link without credentials or token-like
// query parameters.
func safeValidationURL(raw string) string {
	u, err := url.Parse(trimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	q := u.Query()
	for key := range q {
		if strings.Contains(strings.ToLower(key), "token") {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func retryAfter(value string, now time.Time) (time.Duration, bool) {
	value = trimSpace(value)
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(value); err == nil {
		return max(time.Duration(secs)*time.Second, 0), true
	}
	if t, err := http.ParseTime(value); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
