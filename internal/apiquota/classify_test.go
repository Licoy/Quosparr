package apiquota

import "testing"

func TestClassifyStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		exp    expected
		kind   hitKind
		msg    string
	}{
		{"401", 401, "{}", expectGrants, hitUnauthorized, ""},
		{"404", 404, "nope", expectSubscription, hitNotFound, ""},
		{"405", 405, "", expectUsage, hitNotFound, ""},
		{"501", 501, "", expectUsage, hitNotFound, ""},
		{"not json", 200, "not-json", expectUsage, hitUnsupported, ""},
		{"trailing data", 200, `{"total_usage":1} x`, expectUsage, hitUnsupported, ""},
		{"html 200", 200, "  \n<!DOCTYPE html><html>", expectGrants, hitUnsupported, ""},
		{"html 403", 403, "<!DOCTYPE html><html>", expectGrants, hitUnsupported, ""},
		{"json 403", 403, `{"error":{"message":"forbidden"}}`, expectGrants, hitUnauthorized, ""},
		{"502", 502, "down", expectGrants, hitError, "HTTP 502"},
		{"408", 408, "slow", expectToken, hitError, "request timed out"},
		{"429", 429, "busy", expectToken, hitError, "HTTP 429"},
		{"redirect without target", 304, "", expectToken, hitError, "HTTP 304"},
		{"token unauthorized", 200, `{"success":false,"message":"token not found"}`, expectToken, hitUnauthorized, ""},
		{"token unauthorized code flag", 200, `{"code":false,"message":"Invalid Authorization header"}`, expectToken, hitUnauthorized, ""},
		{"token failure", 200, `{"success":false,"message":"service unavailable"}`, expectToken, hitError, "upstream response indicated failure"},
		{"grants error object", 200, `{"error":{"message":"database unavailable"}}`, expectGrants, hitError, "upstream response indicated failure"},
		{"grants error string", 200, `{"error":"boom"}`, expectGrants, hitError, "upstream response indicated failure"},
		{"grants error blank string", 200, `{"error":"  ","total_granted":1}`, expectGrants, hitGrants, ""},
		{"grants error null", 200, `{"error":null,"total_granted":1}`, expectGrants, hitGrants, ""},
		{"grants error empty list", 200, `{"error":[],"total_granted":1}`, expectGrants, hitGrants, ""},
		{"grants error false", 200, `{"error":false,"total_granted":1}`, expectGrants, hitGrants, ""},
		{"grants error number", 200, `{"error":0,"total_granted":1}`, expectGrants, hitError, "upstream response indicated failure"},
		{"status failure", 200, `{"success":false,"message":"status unavailable"}`, expectStatus, hitError, "upstream response indicated failure"},
		{"token invalid data", 200, `{"code":true,"data":{"total_available":"NaN"}}`, expectToken, hitInvalidData, "invalid quota data"},
		{"token no data", 200, `{"code":true}`, expectToken, hitUnsupported, ""},
		{"unrelated json", 200, `{"hello":"world"}`, expectGrants, hitUnsupported, ""},
		{"array json", 200, `[1,2]`, expectStatus, hitUnsupported, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyStatus(tt.status, tt.body, tt.exp)
			if got.kind != tt.kind || got.msg != tt.msg {
				t.Fatalf("kind %v msg %q, want kind %v msg %q", got.kind, got.msg, tt.kind, tt.msg)
			}
		})
	}
}

func TestClassifyParsedPayloads(t *testing.T) {
	if h := classifyStatus(200, `{"total_granted":100,"total_used":40}`, expectGrants); h.kind != hitGrants {
		t.Fatal(h.kind)
	} else {
		wantFloat(t, "remaining", h.grants.remaining, 60)
	}
	if h := classifyStatus(200, `{"hard_limit_usd":5}`, expectSubscription); h.kind != hitSubscription {
		t.Fatal(h.kind)
	}
	if h := classifyStatus(200, `{"total_usage":150}`, expectUsage); h.kind != hitUsage || h.usage.totalUsage != 150 {
		t.Fatal(h.kind)
	}
	if h := classifyStatus(200, `{"success":true,"data":{"quota_per_unit":500000}}`, expectStatus); h.kind != hitStatus {
		t.Fatal(h.kind)
	}
}

func TestClassifyNeverCopiesBody(t *testing.T) {
	for _, status := range []int{200, 401, 403, 404, 500} {
		h := classifyStatus(status, `{"error":"secret-body-text","message":"secret-body-text"}`, expectToken)
		if h.msg == "secret-body-text" {
			t.Fatalf("status %d copied the body", status)
		}
	}
}

func TestClassifyDeepSeekBalance(t *testing.T) {
	body := `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"12.30"}]}`
	h := classifyStatus(200, body, expectDeepSeekBalance)
	if h.kind != hitDeepSeekBalance || h.deepSeek.currency != UnitCNY {
		t.Fatalf("%+v", h)
	}
	wantFloat(t, "total", &h.deepSeek.total, 12.3)

	if h := classifyStatus(200, `{"is_available":false,"balance_infos":[]}`, expectDeepSeekBalance); h.kind != hitInvalidData || h.msg != msgInvalidQuota {
		t.Fatalf("%+v", h)
	}
	if h := classifyStatus(200, `{"hello":"world"}`, expectDeepSeekBalance); h.kind != hitUnsupported {
		t.Fatalf("%+v", h)
	}
	if h := classifyStatus(401, `{"error":"bad key"}`, expectDeepSeekBalance); h.kind != hitUnauthorized {
		t.Fatalf("%+v", h)
	}
}
