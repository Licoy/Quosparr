package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"runtime"
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
	quotaTimeout               = 20 * time.Second
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
	// Now stamps the snapshot; nil means time.Now.
	Now func() time.Time
}

type cloudTier struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
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

// Fetch reads one snapshot with two requests: loadCodeAssist for the plan
// and project, then the quota summary. It never onboards an account and never
// retries; every failure is returned as is.
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
	buckets, err := c.quotaSummary(ctx, base, accessToken, version, body)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	return QuotaSnapshot{
		Tier: info.tier, Project: info.project, Credits: info.credits,
		Source: SourceSummary, Buckets: buckets, UpdatedAt: c.now(),
	}, nil
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

// quotaHeaders identifies the desktop hub subclient: Cloud Code filters
// the quota pools by this identity.
func (c *QuotaClient) quotaHeaders(version string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", "antigravity/hub/"+version+" (aidev_client; os_type="+clientOS(c.goos())+"; arch="+clientArch(c.goarch())+")")
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
