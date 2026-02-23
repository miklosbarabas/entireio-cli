package summarize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// trailTitlePromptTemplate is the prompt used to generate trail titles and descriptions.
//
// Security note: The transcript is wrapped in <transcript> tags to provide clear boundary
// markers. This helps contain any potentially malicious content within the transcript.
const trailTitlePromptTemplate = `Analyze this development session transcript and generate a title and description.

<transcript>
%s
</transcript>

Return a JSON object:
{
  "title": "Short imperative title (max 80 chars)",
  "description": "1-3 sentence description of what was accomplished and why"
}

Guidelines:
- Title: imperative mood, captures core intent (e.g. "Add user authentication flow")
- Description: explain the "what" and "why", not the "how"
- Return ONLY the JSON object`

// trailTitleModel is the model hint for trail title generation.
// Haiku is fast (~1-2s) and cheap — trail titles are simple tasks.
const trailTitleModel = "haiku"

// TrailTitleResult contains the LLM-generated title and description for a trail.
type TrailTitleResult struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// GenerateTrailTitle generates a title and description for a trail using the agent's
// text generation capability. Returns nil if the agent doesn't support text generation.
func GenerateTrailTitle(ctx context.Context, transcriptBytes []byte, filesTouched []string, agentType agent.AgentType) (*TrailTitleResult, error) {
	// Get the active agent and check if it implements TextGenerator
	ag, err := agent.GetByAgentType(agentType)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}
	gen, ok := ag.(agent.TextGenerator)
	if !ok {
		return nil, fmt.Errorf("agent %s does not support text generation", agentType)
	}

	// Build condensed transcript (reuse existing infrastructure)
	condensed, err := BuildCondensedTranscriptFromBytes(transcriptBytes, agentType)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transcript: %w", err)
	}
	if len(condensed) == 0 {
		return nil, errors.New("transcript has no content")
	}

	input := Input{Transcript: condensed, FilesTouched: filesTouched}
	transcriptText := FormatCondensedTranscript(input)

	// Build prompt and call agent's TextGenerator
	prompt := fmt.Sprintf(trailTitlePromptTemplate, transcriptText)
	rawResult, err := gen.GenerateText(ctx, prompt, trailTitleModel)
	if err != nil {
		return nil, fmt.Errorf("text generation failed: %w", err)
	}

	// Parse JSON response (handle markdown code blocks)
	cleaned := extractJSONFromMarkdown(rawResult)
	var result TrailTitleResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("failed to parse trail title JSON: %w (response: %s)", err, cleaned)
	}

	return &result, nil
}
