package apiquota

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	probeTimeout = 8 * time.Second
	maxBodyBytes = 64 << 10
	maxErrorRune = 160
)

// Doer sends one HTTP request; *http.Client satisfies it. The caller owns
// proxy, TLS and redirect policy.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Probe asks the site for its balance. The official DeepSeek API has its own
// balance endpoint and is asked there only; every other site uses the billing
// endpoints, in parallel, with a second round at the origin without /v1 when
// the first finds nothing better than a dashboard estimate. The whole probe is
// bounded by 8 seconds and ctx. The key is trimmed and sent only as a Bearer
// token; it is redacted from the result. An empty key is reported as
// unsupported.
func Probe(ctx context.Context, client Doer, baseURL, apiKey string, now time.Time) Quota {
	if now.IsZero() {
		now = time.Now()
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return quiet(StatusUnsupported, "", "", now)
	}
	if client == nil {
		return quiet(StatusError, errNoClient.Error(), "", now)
	}
	urls, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return quiet(StatusError, err.Error(), "", now)
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	p := prober{client: client, key: key, now: now}
	q := p.run(ctx, urls.OpenAIBase)
	return redactQuota(q, key)
}

type prober struct {
	client Doer
	key    string
	now    time.Time
}

func (p prober) run(ctx context.Context, openAIBase string) Quota {
	if balanceURL, ok := deepSeekBalanceURL(openAIBase); ok {
		r := round{deepSeekURL: balanceURL}
		r.deepSeek = p.fetch(ctx, balanceURL, expectDeepSeekBalance, true)
		return p.finish(r)
	}
	first := p.finish(p.fetchRound(ctx, openAIBase, publicAPIBases(openAIBase), true))
	if !needsFallback(first) {
		return first
	}
	origin, ok := originWithoutV1(openAIBase)
	if !ok || origin == openAIBase {
		return first
	}
	return combine(first, p.finish(p.fetchRound(ctx, origin, nil, false)))
}

func (p prober) finish(r round) Quota {
	r.token = r.normalizeToken()
	return r.interpret(p.now)
}

// fetchRound requests the billing endpoints under apiRoot and, when
// includePublic is set, the public status and token endpoints, all in parallel.
func (p prober) fetchRound(ctx context.Context, apiRoot string, publicBases []string, includePublic bool) round {
	r := round{urls: newBillingURLs(apiRoot, p.now)}
	var wg sync.WaitGroup
	wg.Go(func() { r.grants = p.fetch(ctx, r.urls.creditGrants, expectGrants, true) })
	wg.Go(func() { r.sub = p.fetch(ctx, r.urls.subscription, expectSubscription, true) })
	wg.Go(func() { r.usage = p.fetch(ctx, r.urls.usage, expectUsage, true) })
	if includePublic {
		wg.Go(func() { r.token, r.tokenURL, r.status = p.fetchPublic(ctx, publicBases) })
	}
	wg.Wait()
	return r
}

// fetchPublic tries each public base in order: the unauthenticated status
// document (metadata for raw quota units) and the authenticated token usage.
// Later bases are only tried when their status document is valid, so an
// unrelated site root is not asked about the key.
func (p prober) fetchPublic(ctx context.Context, bases []string) (token hit, tokenURL string, status hit) {
	for i, base := range bases {
		status = p.fetch(ctx, quotaStatusURL(base), expectStatus, false)
		if i > 0 && status.kind != hitStatus {
			continue
		}
		tokenURL = tokenUsageURL(base)
		token = p.fetch(ctx, tokenURL, expectToken, true)
		if token.kind != hitNotFound && token.kind != hitUnsupported {
			return token, tokenURL, status
		}
	}
	return hit{kind: hitUnsupported}, "", hit{kind: hitUnsupported}
}

func (p prober) fetch(ctx context.Context, rawURL string, exp expected, authenticated bool) hit {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return errorHit("invalid request URL")
	}
	req.Header.Set("Accept", "application/json")
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+p.key)
	}
	status, body, err := doRequest(p.client, req, maxBodyBytes)
	if err != nil {
		return errorHit(requestMessage(err, p.key))
	}
	return classifyStatus(status, string(body), exp)
}

// doRequest sends req and reads at most limit bytes of the body.
func doRequest(client Doer, req *http.Request, limit int64) (int, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

// requestMessage turns a transport error into a short message without the key
// or the request URL.
func requestMessage(err error, key string) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "request canceled"
	case errors.Is(err, context.DeadlineExceeded), isTimeout(err):
		return msgTimedOut
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return shorten(redact(err.Error(), key))
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// shorten collapses whitespace and caps the length of an error message.
func shorten(s string) string {
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	if r := []rune(s); len(r) > maxErrorRune {
		return string(r[:maxErrorRune]) + "..."
	}
	return s
}

// redact replaces every occurrence of the key.
func redact(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "<redacted>")
}

// redactQuota is the last line of defense for the text fields of a result.
// The unit comes from the site, so a case-insensitive echo of the key (units
// are upper-cased) is dropped.
func redactQuota(q Quota, key string) Quota {
	q.Endpoint = redact(q.Endpoint, key)
	q.Error = redact(q.Error, key)
	if strings.Contains(strings.ToUpper(q.Unit), strings.ToUpper(key)) {
		q.Unit = ""
	}
	return q
}
