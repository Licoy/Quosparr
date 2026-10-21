package usage

import (
	"bytes"
	"strings"
	"testing"
)

func TestHeadStringReadsCutOffObjects(t *testing.T) {
	head := `{"timestamp":"2026-10-20T00:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_tok`
	for _, c := range []struct {
		head string
		path []string
		want string
		ok   bool
	}{
		{head, []string{"type"}, "event_msg", true},
		{head, []string{"payload", "type"}, "token_count", true},
		{head, []string{"payload", "info", "x"}, "", false},
		{"\xEF\xBB\xBF" + `{"type":"compacted","payload":{"message":"aaa`, []string{"type"}, "compacted", true},
		{`{"pad":"aaa`, []string{"type"}, "", false},
		{`{"type":1}`, []string{"type"}, "", false},
		{`{"type":"a","type":"b"}`, []string{"type"}, "a", true},
		{`{"payload":{"x":1},"type":"y"}`, []string{"payload", "type"}, "", false},
		{`["type"]`, []string{"type"}, "", false},
		{`{"type":"a"}`, nil, "", false},
	} {
		got, ok := HeadString([]byte(c.head), c.path...)
		if got != c.want || ok != c.ok {
			t.Errorf("HeadString(%q, %v) = %q, %v", c.head, c.path, got, ok)
		}
	}
}

func TestLineHeadCopiesTheStart(t *testing.T) {
	buffered := []byte(strings.Repeat("a", LineHeadBytes+10))
	if head := LineHead(buffered, []byte("b")); len(head) != LineHeadBytes || bytes.Contains(head, []byte("b")) {
		t.Fatalf("len = %d", len(head))
	}
	next := []byte("cd")
	head := LineHead([]byte("ab"), next)
	next[0] = 'x'
	if string(head) != "abcd" {
		t.Fatalf("head = %q", head)
	}
}
