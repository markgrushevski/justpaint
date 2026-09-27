// Package gemini is the Gemini provider: one transport to the Generative Language
// API (Client) and the impls built on it — Judge, Critic and Guesser for
// internal/judge's seams, Assist for internal/assist's. The seams own the
// contracts; this package owns only the wire (JUDGE.md §8.1, docs/ASSIST.md §3.2).
package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/markgrushevski/justpaint/server/internal/judge"
)

const (
	geminiMaxAttempts = 3 // 1 try + 2 retries (JUDGE.md §7)
	geminiRetryBase   = 250 * time.Millisecond
	// geminiTemperature is as close to JUDGE.md §9's determinism as a sampling model gets.
	geminiTemperature      = 0.0
	geminiMaxResponseBytes = 1 << 20 // a verdict is a few hundred bytes
	geminiImageMIME        = "image/png"
	// geminiAPIKeyHeader carries the key; a ?key= query string would leak into logs.
	geminiAPIKeyHeader = "x-goog-api-key"
)

var geminiPNGMagic = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// Client is the transport every impl in this package shares: endpoint, credential,
// per-attempt deadline and the JUDGE.md §7 retry policy (JUDGE.md §8.1).
type Client struct {
	label     string // full error prefix: "judge: gemini critic", "assist: gemini"
	apiKey    string
	retryBase time.Duration // a field so tests can shrink it
	endpoint  string
	timeout   time.Duration
	httpc     *http.Client
}

// NewClient builds the shared client. timeout bounds one attempt; a non-positive
// timeout leaves the caller's context as the only bound.
func NewClient(label, apiKey, model, baseURL string, timeout time.Duration) Client {
	return Client{
		label:  label,
		apiKey: apiKey,
		// Only the model id is escaped: ":generateContent" is part of the path grammar.
		endpoint:  fmt.Sprintf("%s/models/%s:generateContent", strings.TrimRight(baseURL, "/"), url.PathEscape(model)),
		timeout:   timeout,
		retryBase: geminiRetryBase,
		httpc:     &http.Client{},
	}
}

// JSONRequest is one text-only structured-output ask.
type JSONRequest struct {
	System string
	// Before and Image, when set, open the user turn: text, then the PNG, then User.
	Before string
	Image  []byte
	User   string
	Schema *Schema
	// MaxOutputTokens of zero means the model's default. A list answer must set it: a
	// thinking model spends the same budget on reasoning, and a truncated answer can
	// still parse (check Output.Finish for FinishTruncated).
	MaxOutputTokens int
}

// GenerateJSON runs one structured-output call at geminiTemperature.
func (c *Client) GenerateJSON(ctx context.Context, req JSONRequest) (Output, error) {
	var parts []geminiPart
	if req.Before != "" {
		parts = append(parts, geminiPart{Text: req.Before})
	}
	if req.Image != nil {
		if err := checkGeminiPNG("image", req.Image); err != nil {
			return Output{}, err
		}
		parts = append(parts, geminiPart{InlineData: geminiPNGPart(req.Image)})
	}
	parts = append(parts, geminiPart{Text: req.User})
	body, err := json.Marshal(geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: req.System}}},
		Contents:          []geminiContent{{Role: "user", Parts: parts}},
		GenerationConfig: geminiGenerationConfig{
			Temperature:      geminiTemperature,
			ResponseMIMEType: "application/json",
			ResponseSchema:   req.Schema,
			MaxOutputTokens:  req.MaxOutputTokens,
		},
	})
	if err != nil {
		return Output{}, fmt.Errorf("%s: encode request: %w", c.label, err)
	}
	return c.generate(ctx, body)
}

// FinishTruncated is the finishReason of an answer cut off at MaxOutputTokens.
const FinishTruncated = "MAX_TOKENS"

// generate posts body under the JUDGE.md §7 retry policy. The body is built once and
// replayed, since the base64 rasters dominate it.
func (c *Client) generate(ctx context.Context, body []byte) (Output, error) {
	var lastErr error
	for attempt := 1; attempt <= geminiMaxAttempts; attempt++ {
		if attempt > 1 {
			if err := geminiBackoff(ctx, c.retryBase, attempt); err != nil {
				return Output{}, fmt.Errorf("%s: retry aborted: %w (last failure: %v)", c.label, err, lastErr)
			}
		}
		out, retryable, err := c.attempt(ctx, body)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !retryable {
			return Output{}, err
		}
		if ctx.Err() != nil {
			return Output{}, fmt.Errorf("%s: call aborted: %w (last failure: %v)", c.label, ctx.Err(), lastErr)
		}
	}
	return Output{}, fmt.Errorf("%s: failed after %d attempts: %w", c.label, geminiMaxAttempts, lastErr)
}

// attempt performs one request; the bool says whether a failure is worth a retry.
func (c *Client) attempt(ctx context.Context, body []byte) (Output, bool, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Output{}, false, fmt.Errorf("%s: build request: %w", c.label, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set(geminiAPIKeyHeader, c.apiKey)

	resp, err := c.httpc.Do(httpReq)
	if err != nil {
		// DNS, TLS, a reset or the per-attempt deadline: all transient.
		return Output{}, true, fmt.Errorf("%s: request failed: %w", c.label, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, geminiMaxResponseBytes))
	if err != nil {
		return Output{}, true, fmt.Errorf("%s: read response: %w", c.label, err)
	}

	if resp.StatusCode != http.StatusOK {
		// 5xx only: a 4xx means our request is wrong, and a 429 will not refill in time.
		return Output{}, resp.StatusCode >= 500, geminiStatusError(c.label, resp.StatusCode, payload)
	}

	out, err := geminiCandidateOutput(c.label, payload)
	if err != nil {
		// Not a blip: a retry at temperature 0 only spends another quota slot.
		return Output{}, false, err
	}
	return out, false, nil
}

// geminiBackoff waits base * 2^(attempt-2), cancellably.
func geminiBackoff(ctx context.Context, base time.Duration, attempt int) error {
	delay := base << (attempt - 2)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func checkGeminiPNG(name string, img []byte) error {
	if len(img) == 0 {
		return fmt.Errorf("judge: gemini: %s is empty", name)
	}
	if !bytes.HasPrefix(img, geminiPNGMagic) {
		return fmt.Errorf("judge: gemini: %s is not a PNG (%d bytes)", name, len(img))
	}
	return nil
}

// --- wire types (Generative Language API generateContent) --------------------

type geminiRequest struct {
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart serves both directions. Thought is response-only; omitempty keeps it off
// requests, where the API rejects it.
type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
	Thought    bool              `json:"thought,omitempty"`
}

type geminiInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"` // base64 PNG bytes
}

type geminiGenerationConfig struct {
	Temperature      float64 `json:"temperature"`
	ResponseMIMEType string  `json:"responseMimeType"`
	ResponseSchema   *Schema `json:"responseSchema,omitempty"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
}

// Schema is the OpenAPI subset the API accepts; an unmodelled field fails the
// whole request. It lacks minItems/maxItems on purpose (JUDGE.md §8.3).
type Schema struct {
	Type        string             `json:"type"`
	Description string             `json:"description,omitempty"`
	Enum        []string           `json:"enum,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Required    []string           `json:"required,omitempty"`
}

type geminiResponse struct {
	Candidates     []geminiCandidate `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

type geminiCandidate struct {
	Content struct {
		Parts []geminiPart `json:"parts"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

type geminiErrorEnvelope struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func geminiPNGPart(img []byte) *geminiInlineData {
	return &geminiInlineData{MIMEType: geminiImageMIME, Data: base64.StdEncoding.EncodeToString(img)}
}

// Output is one candidate's answer. Finish tells a refusal from JSON cut off
// mid-object.
type Output struct {
	Text   string
	Finish string
}

// geminiCandidateOutput extracts the answer from a 200 body, or says why there is none.
func geminiCandidateOutput(label string, payload []byte) (Output, error) {
	var resp geminiResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return Output{}, fmt.Errorf("%s: decode response envelope: %w", label, err)
	}
	if len(resp.Candidates) == 0 {
		// Safety filters drop the whole response, and player drawings make that routine.
		if reason := resp.PromptFeedback.BlockReason; reason != "" {
			return Output{}, fmt.Errorf("%s: request blocked (%s)", label, reason)
		}
		return Output{}, fmt.Errorf("%s: response carried no candidates", label)
	}
	candidate := resp.Candidates[0]
	text := geminiCandidateText(candidate)
	if text == "" {
		return Output{}, fmt.Errorf("%s: candidate carried no text (finishReason %q)", label, candidate.FinishReason)
	}
	return Output{Text: text, Finish: candidate.FinishReason}, nil
}

// clampText trims over-long prose to its cap instead of failing the answer. HTTPJudge
// rejects the same overrun on purpose (JUDGE.md §8.1).
func clampText(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	// End on a word when a space is near the cap.
	cut := runes[:limit-1]
	if i := lastIndexRune(cut, ' '); i > limit-40 {
		cut = cut[:i]
	}
	return strings.TrimRight(string(cut), " ,;:") + "…"
}

func lastIndexRune(runes []rune, target rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == target {
			return i
		}
	}
	return -1
}

// geminiCandidateText joins the answer parts, skipping thinking summaries.
func geminiCandidateText(c geminiCandidate) string {
	var b strings.Builder
	for _, p := range c.Content.Parts {
		if p.Thought {
			continue
		}
		b.WriteString(p.Text)
	}
	return strings.TrimSpace(b.String())
}

// geminiStatusError keeps the API's own message, which is what tells quota, unknown
// model and bad key apart; truncated, since the body may be a proxy's HTML page.
func geminiStatusError(label string, status int, payload []byte) error {
	var env geminiErrorEnvelope
	_ = json.Unmarshal(payload, &env)
	detail := geminiTruncate(env.Error.Message, 300)
	if detail == "" {
		detail = geminiTruncate(string(payload), 300)
	}
	statusLabel := fmt.Sprintf("HTTP %d", status)
	if env.Error.Status != "" {
		statusLabel += " " + env.Error.Status
	}
	if status == http.StatusTooManyRequests {
		return fmt.Errorf("%s: %w (%s): %s", label, judge.ErrQuotaExhausted, statusLabel, detail)
	}
	return fmt.Errorf("%s: %s: %s", label, statusLabel, detail)
}

func geminiTruncate(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= limit {
		return string(r)
	}
	return string(r[:limit]) + "..."
}
