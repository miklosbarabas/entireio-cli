package strategy

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

func TestGenerateContextFromPrompts_CJKTruncation(t *testing.T) {
	t.Parallel()

	// 600 CJK characters exceeds the 500-rune truncation limit.
	prompt := strings.Repeat("あ", 600)

	result := generateContextFromPrompts([]string{prompt})

	if !utf8.Valid(result) {
		t.Error("generateContextFromPrompts produced invalid UTF-8 when truncating a CJK prompt")
	}

	resultStr := string(result)
	if !strings.Contains(resultStr, "...") {
		t.Error("expected truncated CJK prompt to contain '...' suffix")
	}
	// Should not contain more than 500 CJK characters
	if strings.Contains(resultStr, strings.Repeat("あ", 501)) {
		t.Error("CJK prompt was not truncated")
	}
}

func TestGenerateContextFromPrompts_EmojiTruncation(t *testing.T) {
	t.Parallel()

	// 600 emoji exceeds the 500-rune truncation limit.
	prompt := strings.Repeat("🎉", 600)

	result := generateContextFromPrompts([]string{prompt})

	if !utf8.Valid(result) {
		t.Error("generateContextFromPrompts produced invalid UTF-8 when truncating an emoji prompt")
	}

	resultStr := string(result)
	if !strings.Contains(resultStr, "...") {
		t.Error("expected truncated emoji prompt to contain '...' suffix")
	}
}

func TestGenerateContextFromPrompts_ASCIITruncation(t *testing.T) {
	t.Parallel()

	// Pure ASCII: should truncate at 500 runes with "..." suffix.
	prompt := strings.Repeat("a", 600)

	result := generateContextFromPrompts([]string{prompt})

	if !utf8.Valid(result) {
		t.Error("generateContextFromPrompts produced invalid UTF-8 when truncating an ASCII prompt")
	}

	resultStr := string(result)
	if !strings.Contains(resultStr, "...") {
		t.Error("expected truncated prompt to contain '...' suffix")
	}

	if strings.Contains(resultStr, strings.Repeat("a", 501)) {
		t.Error("prompt was not truncated")
	}
}

func TestGenerateContextFromPrompts_ShortCJKNotTruncated(t *testing.T) {
	t.Parallel()

	// 200 CJK characters is under the 500-rune limit, should not be truncated.
	prompt := strings.Repeat("あ", 200)

	result := generateContextFromPrompts([]string{prompt})

	if !utf8.Valid(result) {
		t.Error("generateContextFromPrompts produced invalid UTF-8")
	}

	resultStr := string(result)
	if strings.Contains(resultStr, "...") {
		t.Error("short CJK prompt should not be truncated")
	}
}

// droidMessage builds a Droid JSONL "message" line with the given id, role, and optional usage.
func droidMessage(t *testing.T, id, role string, usage map[string]int) string {
	t.Helper()
	inner := map[string]interface{}{
		"role":    role,
		"content": []interface{}{},
	}
	if usage != nil {
		inner["id"] = id
		inner["usage"] = usage
	}
	msg, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("failed to marshal inner message: %v", err)
	}
	line := map[string]interface{}{
		"type":    "message",
		"id":      id,
		"message": json.RawMessage(msg),
	}
	b, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("failed to marshal droid line: %v", err)
	}
	return string(b)
}

func TestCalculateTokenUsage_DroidStartOffsetSkipsNonMessageLines(t *testing.T) {
	t.Parallel()

	// Build a Droid transcript with non-message entries interspersed:
	// Line 0: session_start (non-message)
	// Line 1: user message (no tokens)
	// Line 2: assistant message with 10 input, 20 output tokens
	// Line 3: session_event (non-message)
	// Line 4: assistant message with 5 input, 30 output tokens
	transcript := "" +
		`{"type":"session_start","id":"s1"}` + "\n" +
		droidMessage(t, "m1", "user", nil) + "\n" +
		droidMessage(t, "m2", "assistant", map[string]int{
			"input_tokens": 10, "output_tokens": 20,
		}) + "\n" +
		`{"type":"session_event","data":"heartbeat"}` + "\n" +
		droidMessage(t, "m3", "assistant", map[string]int{
			"input_tokens": 5, "output_tokens": 30,
		}) + "\n"

	data := []byte(transcript)

	// With startOffset=0: should count all messages (m2 + m3)
	usageAll := calculateTokenUsage(agent.AgentTypeFactoryAIDroid, data, 0)
	if usageAll.InputTokens != 15 {
		t.Errorf("startOffset=0: InputTokens = %d, want 15", usageAll.InputTokens)
	}
	if usageAll.OutputTokens != 50 {
		t.Errorf("startOffset=0: OutputTokens = %d, want 50", usageAll.OutputTokens)
	}
	if usageAll.APICallCount != 2 {
		t.Errorf("startOffset=0: APICallCount = %d, want 2", usageAll.APICallCount)
	}

	// With startOffset=3: skip lines 0-2 (session_start, m1, m2).
	// Only line 3 (session_event, filtered) and line 4 (m3) remain.
	// Should count only m3's tokens.
	usageFrom3 := calculateTokenUsage(agent.AgentTypeFactoryAIDroid, data, 3)
	if usageFrom3.InputTokens != 5 {
		t.Errorf("startOffset=3: InputTokens = %d, want 5", usageFrom3.InputTokens)
	}
	if usageFrom3.OutputTokens != 30 {
		t.Errorf("startOffset=3: OutputTokens = %d, want 30", usageFrom3.OutputTokens)
	}
	if usageFrom3.APICallCount != 1 {
		t.Errorf("startOffset=3: APICallCount = %d, want 1", usageFrom3.APICallCount)
	}

	// Regression: using the OLD buggy code would have parsed all messages (ignoring
	// non-message entries), producing [m1, m2, m3], then sliced at index 3 which
	// is out of bounds — returning all tokens instead of just m3's.
	// With startOffset=1: skip only line 0 (session_start).
	// Lines 1 (m1), 2 (m2), 3 (session_event, filtered), 4 (m3) remain.
	usageFrom1 := calculateTokenUsage(agent.AgentTypeFactoryAIDroid, data, 1)
	if usageFrom1.InputTokens != 15 {
		t.Errorf("startOffset=1: InputTokens = %d, want 15", usageFrom1.InputTokens)
	}
	if usageFrom1.APICallCount != 2 {
		t.Errorf("startOffset=1: APICallCount = %d, want 2", usageFrom1.APICallCount)
	}
}

// Verify that startOffset beyond transcript length returns empty usage.
func TestCalculateTokenUsage_DroidStartOffsetBeyondEnd(t *testing.T) {
	t.Parallel()

	data := []byte(
		`{"type":"session_start","id":"s1"}` + "\n" +
			droidMessage(t, "m1", "assistant", map[string]int{
				"input_tokens": 10, "output_tokens": 20,
			}) + "\n",
	)

	usage := calculateTokenUsage(agent.AgentTypeFactoryAIDroid, data, 100)
	if usage.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", usage.InputTokens)
	}
	if usage.APICallCount != 0 {
		t.Errorf("APICallCount = %d, want 0", usage.APICallCount)
	}
}
