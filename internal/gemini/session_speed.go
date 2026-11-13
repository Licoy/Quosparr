package gemini

import (
	"context"
	"sort"
	"time"

	"github.com/licoy/quosparr/internal/speed"
)

// ScanSpeed reads the model speed samples of the newest speed.SessionLimit
// conversations updated within speed.Window before now, newest first. A
// call gives its time to first token, and its response tokens (thinking
// excluded) over the streaming time as the output part. A damaged database
// is skipped.
func (s SessionScanner) ScanSpeed(ctx context.Context, now time.Time) ([]speed.Session, error) {
	entries, _, err := s.sessionList(ctx)
	if err != nil {
		return nil, err
	}
	cutoff := now.Add(-speed.Window)
	var result []speed.Session
	for _, entry := range entries {
		if len(result) >= speed.SessionLimit {
			break
		}
		if entry.updatedAt().Before(cutoff) {
			continue
		}
		conv, err := s.sessionConversationOf(ctx, entry)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		result = append(result, speed.Session{
			ID: entry.id, Title: entry.title(), ProjectName: entry.projectName(),
			UpdatedAt: entry.updatedAt(), Samples: sessionSamples(conv, entry.updatedAt()),
		})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func sessionSamples(conv sessionConversation, fallback time.Time) []speed.Sample {
	var samples []speed.Sample
	for _, rec := range conv.records {
		if !rec.timed {
			continue
		}
		at := rec.at
		if at.IsZero() {
			at = fallback
		}
		sample := speed.Sample{At: at, Model: rec.model}
		if rec.ttft > 0 && rec.ttft <= speed.MaxTTFT {
			sample.TTFT, sample.HasTTFT = rec.ttft, true
		}
		if rec.usage.response > 0 && rec.stream > 0 {
			sample.OutputTokens, sample.Decode = rec.usage.response, rec.stream
		}
		if sample.HasTTFT || sample.OutputTokens > 0 {
			samples = append(samples, sample)
		}
	}
	return samples
}
