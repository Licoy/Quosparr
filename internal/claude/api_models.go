package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// AppliedAPIModels records only the model values last applied by Quosparr,
// after expanding the context choice, never credentials or the entire profile.
type AppliedAPIModels struct {
	Model       string `json:"model,omitempty"`
	OpusModel   string `json:"opusModel,omitempty"`
	SonnetModel string `json:"sonnetModel,omitempty"`
	HaikuModel  string `json:"haikuModel,omitempty"`
	FableModel  string `json:"fableModel,omitempty"`
}

func appliedAPIModels(e APIEnv) *AppliedAPIModels {
	e = trimmedAPIEnv(e)
	return &AppliedAPIModels{e.Model, e.OpusModel, e.SonnetModel, e.HaikuModel, e.FableModel}
}

func (a ManagedAccount) appliedAPIEnv(key string) APIEnv {
	e := a.API.Env(key)
	if p := a.AppliedAPIModels; p != nil {
		e.Model, e.OpusModel, e.SonnetModel, e.HaikuModel, e.FableModel = p.Model, p.OpusModel, p.SonnetModel, p.HaikuModel, p.FableModel
	}
	return e
}

// apiSettingsOutdated compares only values an explicit application would change,
// so an empty desired model does not claim ownership of unrelated manual values.
func apiSettingsOutdated(data []byte, want APIEnv, previous APIEnv) (bool, error) {
	want, previous = trimmedAPIEnv(want), trimmedAPIEnv(previous)
	root, vars, err := parseSettings(data)
	if err != nil {
		return false, err
	}
	if stringField(vars[envBaseURL]) != want.BaseURL {
		return true, nil
	}
	differs := func(live, next, old string) bool {
		if next != "" {
			return live != next
		}
		return old != "" && live == old
	}
	if differs(stringField(root["model"]), want.Model, previous.Model) {
		return true, nil
	}
	if _, exists := vars["ANTHROPIC_MODEL"]; exists && want.Model != "" {
		return true, nil
	}
	for _, f := range mappingFields {
		if differs(stringField(vars[f.key]), f.get(want), f.get(previous)) {
			return true, nil
		}
	}
	return false, nil
}

// appliedAPIIdentity identifies the relay login without retaining the key.
// The domain and URL length make the hash input unambiguous across identities.
func appliedAPIIdentity(base, key string) (string, error) {
	normalized, err := NormalizeAPIBaseURL(base)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte("quosparr-claude-applied-models-v1\x00" + strconv.Itoa(len(normalized)) + ":" + normalized + key))
	return hex.EncodeToString(sum[:]), nil
}
