package apiquota

import (
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

type expected int

const (
	expectGrants expected = iota
	expectSubscription
	expectUsage
	expectToken
	expectStatus
	expectDeepSeekBalance
)

type hitKind int

const (
	// hitUnsupported is the zero value: nothing usable was found.
	hitUnsupported hitKind = iota
	hitNotFound
	hitUnauthorized
	hitError
	hitInvalidData
	hitGrants
	hitSubscription
	hitUsage
	hitToken
	hitStatus
	hitDeepSeekBalance
)

// hit is the classified answer of one endpoint.
type hit struct {
	kind     hitKind
	msg      string // hitError and hitInvalidData only
	grants   grants
	sub      subscription
	usage    usage
	token    tokenUsage
	status   quotaStatus
	deepSeek deepSeekBalance
}

func errorHit(msg string) hit { return hit{kind: hitError, msg: msg} }

const (
	msgTimedOut        = "request timed out"
	msgUpstreamFailure = "upstream response indicated failure"
	msgInvalidQuota    = "invalid quota data"
)

// classifyStatus maps an HTTP status and body to a hit. Bodies are never
// copied into the result.
func classifyStatus(status int, body string, exp expected) hit {
	switch {
	case status == http.StatusRequestTimeout:
		return errorHit(msgTimedOut)
	case status == http.StatusUnauthorized:
		return hit{kind: hitUnauthorized}
	case status == http.StatusForbidden:
		if looksLikeHTML(body) {
			return hit{kind: hitUnsupported}
		}
		return hit{kind: hitUnauthorized}
	case status == http.StatusNotFound, status == http.StatusMethodNotAllowed, status == http.StatusNotImplemented:
		return hit{kind: hitNotFound}
	case status >= 200 && status < 300:
		return classifySuccess(body, exp)
	}
	return errorHit("HTTP " + strconv.Itoa(status))
}

func classifySuccess(body string, exp expected) hit {
	if looksLikeHTML(body) {
		return hit{kind: hitUnsupported}
	}
	value, ok := decodeJSON(body)
	if !ok {
		return hit{kind: hitUnsupported}
	}
	if exp == expectToken && tokenResponseUnauthorized(value) {
		return hit{kind: hitUnauthorized}
	}
	if responseIndicatesFailure(value) {
		return errorHit(msgUpstreamFailure)
	}
	return parseExpected(value, exp)
}

func parseExpected(value any, exp expected) hit {
	switch exp {
	case expectGrants:
		if g, ok := parseCreditGrants(value); ok {
			return hit{kind: hitGrants, grants: g}
		}
	case expectSubscription:
		if s, ok := parseSubscription(value); ok {
			return hit{kind: hitSubscription, sub: s}
		}
	case expectUsage:
		if u, ok := parseUsage(value); ok {
			return hit{kind: hitUsage, usage: u}
		}
	case expectToken:
		if t, ok := parseTokenUsage(value); ok {
			return hit{kind: hitToken, token: t}
		}
		if o, ok := asObject(value); ok {
			if _, isObject := asObject(o["data"]); isObject {
				return hit{kind: hitInvalidData, msg: msgInvalidQuota}
			}
		}
	case expectStatus:
		if s, ok := parseQuotaStatus(value); ok {
			return hit{kind: hitStatus, status: s}
		}
	case expectDeepSeekBalance:
		if b, ok := parseDeepSeekBalance(value); ok {
			return hit{kind: hitDeepSeekBalance, deepSeek: b}
		}
		if o, ok := asObject(value); ok {
			if _, present := o["balance_infos"]; present {
				return hit{kind: hitInvalidData, msg: msgInvalidQuota}
			}
		}
	}
	return hit{kind: hitUnsupported}
}

func looksLikeHTML(body string) bool {
	trimmed := strings.TrimLeftFunc(body, unicode.IsSpace)
	return hasPrefixFold(trimmed, "<!doctype") || hasPrefixFold(trimmed, "<html")
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && asciiLower(s[:len(prefix)]) == prefix
}

func failedFlag(o object) bool {
	if b, ok := fieldBool(o, "success"); ok && !b {
		return true
	}
	b, ok := fieldBool(o, "code")
	return ok && !b
}

var unauthorizedFragments = []string{
	"authorization", "bearer", "token not found", "invalid token", "unauthorized", "forbidden",
}

// tokenResponseUnauthorized detects the 200-with-failure answer some sites
// give a missing or unknown key on the token endpoint.
func tokenResponseUnauthorized(value any) bool {
	o, ok := asObject(value)
	if !ok || !failedFlag(o) {
		return false
	}
	msg, _ := fieldString(o, "message")
	msg = asciiLower(msg)
	for _, fragment := range unauthorizedFragments {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}

func responseIndicatesFailure(value any) bool {
	o, ok := asObject(value)
	if !ok {
		return false
	}
	return failedFlag(o) || errorFieldSet(o["error"])
}

func errorFieldSet(v any) bool {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x) != ""
	case []any:
		return len(x) > 0
	case object:
		return len(x) > 0
	case bool:
		return x
	case nil:
		return false
	}
	return true // number
}
