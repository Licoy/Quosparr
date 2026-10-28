package grok

import (
	"encoding/base64"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// TierKind is a Grok / SuperGrok subscription tier, used for badges and plan
// labels. The mapping matches the official Grok Build CLI (jwt_tier_claim and
// normalize_tier).
//
// Sources, in the order the billing client applies them: the chat-proxy
// /v1/settings "subscription_tier_display", the billing response
// "subscription_tier", and the numeric "tier" claim of the OAuth access token.
type TierKind int

// Subscription tiers.
const (
	TierFree TierKind = iota
	TierSuperGrok
	TierSuperGrokHeavy
	TierSuperGrokLite
	TierSuperGrokPlus
	TierXBasic
	TierXPremium
	TierXPremiumPlus
	TierAPIKey
	TierUnknown
	TierXSubscriptionPlus
	TierXSubscriptionPremium
	TierXSubscriptionSuper
	TierXSubscriptionUltra
)

// DisplayName is the canonical English label shown on badges. Brand names
// stay unlocalized.
func (k TierKind) DisplayName() string {
	switch k {
	case TierFree:
		return "Free"
	case TierSuperGrok:
		return "SuperGrok"
	case TierSuperGrokHeavy:
		return "SuperGrok Heavy"
	case TierSuperGrokLite:
		return "SuperGrok Lite"
	case TierSuperGrokPlus:
		return "SuperGrok Plus"
	case TierXBasic:
		return "X Basic"
	case TierXPremium:
		return "X Premium"
	case TierXPremiumPlus:
		return "X Premium+"
	case TierXSubscriptionPlus:
		return "X · Plus"
	case TierXSubscriptionPremium:
		return "X · Premium"
	case TierXSubscriptionSuper:
		return "X · Super"
	case TierXSubscriptionUltra:
		return "X · Ultra"
	case TierAPIKey:
		return "API Key"
	default:
		return "Unknown"
	}
}

// TierResolve maps an API key, JWT claim name or display string to a tier.
// Empty input and anything unrecognized is TierUnknown.
func TierResolve(raw string) TierKind {
	if tier := tierResolveExact(raw); tier != TierUnknown {
		return tier
	}
	return tierResolveFuzzy(tierNormalizeKey(raw))
}

func tierResolveExact(raw string) TierKind {
	trimmed, ok := billingNonEmpty(raw)
	if !ok {
		return TierUnknown
	}
	if tier := subscriptionNamedTier(trimmed); tier != TierUnknown {
		return tier
	}
	key := tierNormalizeKey(trimmed)
	switch key {
	case "free":
		return TierFree
	case "supergrok", "grokpro":
		return TierSuperGrok
	case "supergrok_heavy", "supergrokpro", "supergrokheavy", "heavy":
		return TierSuperGrokHeavy
	case "supergrok_lite", "supergroklite":
		return TierSuperGrokLite
	case "supergrok_plus", "supergrokplus":
		return TierSuperGrokPlus
	case "x_basic", "xbasic":
		return TierXBasic
	case "x_premium", "xpremium":
		return TierXPremium
	case "x_premium_plus", "xpremiumplus", "x_premium_plus_", "premium_plus", "premium_plus_":
		return TierXPremiumPlus
	case "api_key", "apikey", "api":
		return TierAPIKey
	case "unknown":
		return TierUnknown
	}
	return TierUnknown
}

// TierDisplayName is the friendly label for UI tags. A value that is not a
// known tier (for example "Tier 9" from an unknown JWT claim) is returned
// trimmed, and an empty value reports false.
func TierDisplayName(raw string) (string, bool) {
	trimmed, ok := billingNonEmpty(raw)
	if !ok {
		return "", false
	}
	if tier := TierResolve(trimmed); tier != TierUnknown {
		return tier.DisplayName(), true
	}
	return trimmed, true
}

// TierFromJWTClaim maps the numeric prod_auth.SubscriptionTier claim.
func TierFromJWTClaim(tier uint64) TierKind {
	switch tier {
	case 0:
		return TierFree
	case 1:
		return TierSuperGrok
	case 2:
		return TierXBasic
	case 3:
		return TierXPremium
	case 4:
		return TierXPremiumPlus
	case 5:
		return TierSuperGrokHeavy
	case 6:
		return TierSuperGrokLite
	case 7:
		return TierSuperGrokPlus
	}
	return TierUnknown
}

// TierDisplayNameFromJWTClaim maps a numeric claim to its label, "Tier N" for
// an unmapped number.
func TierDisplayNameFromJWTClaim(tier uint64) string {
	if resolved := TierFromJWTClaim(tier); resolved != TierUnknown {
		return resolved.DisplayName()
	}
	return "Tier " + strconv.FormatUint(tier, 10)
}

// TierDisplayNameFromAccessToken decodes the unsigned JWT payload and maps its
// numeric "tier" claim. It reports false when the token has no usable claim.
func TierDisplayNameFromAccessToken(token string) (string, bool) {
	claim, ok := TierJWTClaim(token)
	if !ok {
		return "", false
	}
	return TierDisplayNameFromJWTClaim(claim), true
}

// TierFromAccessToken is TierDisplayNameFromAccessToken returning the tier.
func TierFromAccessToken(token string) (TierKind, bool) {
	claim, ok := TierJWTClaim(token)
	if !ok {
		return TierUnknown, false
	}
	return TierFromJWTClaim(claim), true
}

// TierJWTClaim reads the "tier" claim of the token's payload segment. The
// claim may be a JSON number or boolean (true is 1), or a decimal string.
// Negative values are rejected.
func TierJWTClaim(token string) (uint64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0, false
	}
	data, ok := tierBase64URLDecode(parts[1])
	if !ok {
		return 0, false
	}
	root, err := parseLooseJSON([]byte(data))
	if err != nil || root.kind != jsonObject {
		return 0, false
	}
	node := root.get("tier")
	if node == nil {
		return 0, false
	}
	switch node.kind {
	case jsonBool:
		if node.b {
			return 1, true
		}
		return 0, true
	case jsonNumber:
		v, ok := tierNumberInt64(node.s)
		if !ok || v < 0 {
			return 0, false
		}
		return uint64(v), true
	case jsonString:
		return tierParseUInt64(node.s)
	}
	return 0, false
}

// tierParseUInt64 is Swift's UInt64(String): an optional sign, then decimal
// digits only ("+5" and "-0" parse, " 5" and "5.0" do not).
func tierParseUInt64(s string) (uint64, bool) {
	body := s
	if body != "" && (body[0] == '+' || body[0] == '-') {
		neg := body[0] == '-'
		body = body[1:]
		if neg {
			for _, c := range body {
				if c != '0' {
					return 0, false
				}
			}
		}
	}
	if body == "" {
		return 0, false
	}
	for i := 0; i < len(body); i++ {
		if body[i] < '0' || body[i] > '9' {
			return 0, false
		}
	}
	v, err := strconv.ParseUint(body, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// tierNumberInt64 reproduces NSNumber.int64Value for a JSON number token as
// the system parser builds it: integers keep their exact value (one that does
// not fit 64 bits wraps around), fractions truncate toward zero and large
// doubles saturate. Numbers with more than 17 significant digits arrive as
// decimals in the system parser; their handling beyond plain truncation is
// approximated.
func tierNumberInt64(tok string) (int64, bool) {
	if !strings.ContainsAny(tok, ".eE") {
		n, ok := new(big.Int).SetString(tok, 10)
		if !ok {
			return 0, false
		}
		if n.IsInt64() {
			return n.Int64(), true
		}
		if n.IsUint64() {
			return int64(n.Uint64()), true // wraps to a negative value
		}
		limit := new(big.Int).Lsh(big.NewInt(1), 128)
		if n.CmpAbs(limit) >= 0 {
			return 0, false
		}
		mask := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1))
		low := new(big.Int).And(new(big.Int).Abs(n), mask).Uint64()
		if n.Sign() < 0 {
			low = -low
		}
		return int64(low), true
	}
	if tierSignificantDigits(tok) >= 20 {
		return 0, true
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil || math.IsNaN(f) {
		return 0, false
	}
	if f >= math.MaxInt64 {
		return math.MaxInt64, true
	}
	if f <= math.MinInt64 {
		return math.MinInt64, true
	}
	return int64(f), true
}

// tierSignificantDigits counts the digits of the mantissa without leading
// zeros.
func tierSignificantDigits(tok string) int {
	mantissa := tok
	if i := strings.IndexAny(mantissa, "eE"); i >= 0 {
		mantissa = mantissa[:i]
	}
	mantissa = strings.TrimLeft(strings.TrimPrefix(mantissa, "-"), "0.")
	return len(strings.ReplaceAll(mantissa, ".", ""))
}

// tierBase64URLDecode decodes a JWT segment the way Foundation's
// Data(base64Encoded:) does after the app converts the URL alphabet and pads
// to a multiple of four characters: padding is optional and excess "=" is
// tolerated, trailing bits are not checked, and any other stray character
// (whitespace included) is an error.
func tierBase64URLDecode(segment string) (string, bool) {
	s := strings.NewReplacer("-", "+", "_", "/").Replace(segment)
	body := strings.TrimRight(s, "=")
	if strings.ContainsAny(body, "=") || len(body)%4 == 1 {
		return "", false
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/':
		default:
			return "", false
		}
	}
	out, err := base64.RawStdEncoding.DecodeString(body)
	if err != nil {
		return "", false
	}
	return string(out), true
}

func tierResolveFuzzy(key string) TierKind {
	has := func(sub string) bool { return strings.Contains(key, sub) }
	switch {
	case has("heavy"):
		return TierSuperGrokHeavy
	case has("lite") && has("supergrok"):
		return TierSuperGrokLite
	case has("plus") && has("supergrok"):
		return TierSuperGrokPlus
	case has("premium") && (has("plus") || strings.HasSuffix(key, "+")):
		return TierXPremiumPlus
	case has("premium"):
		return TierXPremium
	case has("basic") && has("x"):
		return TierXBasic
	case has("supergrok") || has("grokpro"):
		return TierSuperGrok
	case has("api"):
		return TierAPIKey
	case has("free"):
		return TierFree
	}
	return TierUnknown
}

func tierNormalizeKey(value string) string {
	v := strings.ToLower(billingTrim(value))
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "+", "_plus")
	return strings.ReplaceAll(v, "-", "_")
}
