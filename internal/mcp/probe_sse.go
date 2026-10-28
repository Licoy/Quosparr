package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

func probeReadFrames(ctx context.Context, frames <-chan probeFrame, id int, method string) (json.RawMessage, error) {
	for count := 0; count < 1000; count++ {
		select {
		case <-ctx.Done():
			return nil, probeContextError(ctx, probeStage(method), ctx.Err())
		case frame, ok := <-frames:
			if !ok {
				return nil, probeErr("invalid_response", probeStage(method))
			}
			if frame.err != nil {
				return nil, frame.err
			}
			if frame.event != "message" && frame.event != "" {
				continue
			}
			var env probeEnvelope
			if !probeJSONSafe([]byte(frame.data)) || json.Unmarshal([]byte(frame.data), &env) != nil || env.JSONRPC != "2.0" {
				return nil, probeErr("invalid_response", probeStage(method))
			}
			if env.Method != "" && env.ID != nil {
				return nil, probeErr("protocol", probeStage(method))
			}
			if env.Method != "" && env.ID == nil {
				continue
			}
			if string(env.ID) != strconv.Itoa(id) {
				continue
			}
			return probeDecode([]byte(frame.data), id, method)
		}
	}
	return nil, probeErr("response_too_large", probeStage(method))
}
func probeSSE(ctx context.Context, reader io.Reader, frames chan<- probeFrame) {
	defer close(frames)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), probeLimit)
	event := ""
	var data strings.Builder
	total := 0
	send := func(f probeFrame) bool {
		select {
		case frames <- f:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		total += len(line) + 1
		if total > probeLimit {
			send(probeFrame{err: probeErr("response_too_large", "connect")})
			return
		}
		if line == "" {
			if data.Len() > 0 {
				if !send(probeFrame{event: event, data: strings.TrimSuffix(data.String(), "\n")}) {
					return
				}
			}
			event = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if scanner.Err() != nil {
		send(probeFrame{err: probeErr("response_too_large", "connect")})
	}
}
