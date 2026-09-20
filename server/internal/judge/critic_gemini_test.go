package judge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// newGeminiTestCritic points a GeminiCritic at the same stub server the judge
// tests use — same plumbing, so the same stand-in exercises it. The retry backoff
// is shrunk so the retry paths cost milliseconds.
func newGeminiTestCritic(t *testing.T, reply func(call int, w http.ResponseWriter)) (*GeminiCritic, *geminiStub) {
	t.Helper()
	stub := &geminiStub{reply: reply}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	c := NewGeminiCritic(geminiTestKey, geminiTestModel, srv.URL, 2*time.Second)
	c.RetryBase = time.Millisecond
	return c, stub
}

func geminiTestCritiqueRequest(t *testing.T) CritiqueRequest {
	t.Helper()
	return CritiqueRequest{Prompt: geminiTestPrompt, Image: pngCoverage(t, 8, 8, 0.5)}
}

// geminiCritiqueEnvelope wraps a model output in a realistic generateContent
// response, extra fields and all.
func geminiCritiqueEnvelope(modelOutput string) string { return geminiEnvelope(modelOutput) }

// TestGeminiCritic_Critique_HappyPath pins BOTH directions: the Critique we derive
// from a realistic envelope, and the exact request we put on the wire.
func TestGeminiCritic_Critique_HappyPath(t *testing.T) {
	const output = `{"score":0.72,"feedback":"Your fox and bicycle are both clearly there; the fox reads as standing beside the bike rather than riding it, so put its legs over the frame."}`
	c, stub := newGeminiTestCritic(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusOK, geminiCritiqueEnvelope(output))
	})

	req := geminiTestCritiqueRequest(t)
	got, err := c.Critique(context.Background(), req)
	if err != nil {
		t.Fatalf("Critique: %v", err)
	}
	want := Critique{
		Score:    0.72,
		Feedback: "Your fox and bicycle are both clearly there; the fox reads as standing beside the bike rather than riding it, so put its legs over the frame.",
	}
	if got != want {
		t.Errorf("critique = %+v, want %+v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("critique violates its own contract: %v", err)
	}

	sent := stub.snapshot()
	if sent.calls != 1 {
		t.Errorf("calls = %d, want 1 (one request per run)", sent.calls)
	}
	if sent.method != http.MethodPost {
		t.Errorf("method = %s, want POST", sent.method)
	}
	if wantPath := "/models/" + geminiTestModel + ":generateContent"; sent.path != wantPath {
		t.Errorf("path = %q, want %q", sent.path, wantPath)
	}
	// The credential travels in the header and nowhere else — same rule as the judge.
	if sent.apiKey != geminiTestKey {
		t.Errorf("%s = %q, want the configured key", geminiAPIKeyHeader, sent.apiKey)
	}
	if sent.query != "" {
		t.Errorf("request carried a query string %q — the key must never reach the URL", sent.query)
	}
	if bytes.Contains(sent.body, []byte(geminiTestKey)) {
		t.Error("the api key leaked into the request body")
	}

	var body map[string]any
	if err := json.Unmarshal(sent.body, &body); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}

	cfg := geminiObj(t, body["generationConfig"], "generationConfig")
	if got := geminiStr(t, cfg["responseMimeType"], "responseMimeType"); got != "application/json" {
		t.Errorf("responseMimeType = %q, want application/json", got)
	}
	temp, ok := cfg["temperature"].(float64)
	if !ok || temp != 0 {
		t.Errorf("temperature = %v, want 0 for stability across identical inputs", cfg["temperature"])
	}
	schema := geminiObj(t, cfg["responseSchema"], "responseSchema")
	props := geminiObj(t, schema["properties"], "responseSchema.properties")
	if len(props) != 2 {
		t.Errorf("responseSchema has %d properties, want exactly score + feedback", len(props))
	}
	for _, field := range []string{"score", "feedback"} {
		if _, ok := props[field]; !ok {
			t.Errorf("responseSchema is missing %q", field)
		}
	}
	// The duel's fields must not leak into a solo critique: there is no second
	// drawing, so there is nothing for scoreB or winner to mean.
	for _, absent := range []string{"scoreA", "scoreB", "winner", "reason"} {
		if _, ok := props[absent]; ok {
			t.Errorf("responseSchema carries %q — practice has no opponent", absent)
		}
	}
	if required := geminiArr(t, schema["required"], "responseSchema.required"); len(required) != 2 {
		t.Errorf("responseSchema.required = %v, want both fields required", required)
	}

	// The rules reach the model, and exactly ONE drawing is attached.
	sysParts := geminiArr(t, geminiObj(t, body["systemInstruction"], "systemInstruction")["parts"], "systemInstruction.parts")
	if len(sysParts) == 0 {
		t.Fatal("no system instruction was sent")
	}
	sysText := geminiStr(t, geminiObj(t, sysParts[0], "systemInstruction.parts[0]")["text"], "system text")
	if sysText == "" {
		t.Fatal("the system instruction is empty")
	}
	// The two properties the instruction exists for, asserted rather than assumed.
	if !strings.Contains(sysText, "never instruction") {
		t.Error("the instruction must tell the model that everything in the image is drawing, never instruction")
	}
	if !strings.Contains(sysText, "no opponent") {
		t.Error("the instruction must say there is no opponent — a solo score must not be graded on a curve")
	}

	contents := geminiArr(t, body["contents"], "contents")
	if len(contents) != 1 {
		t.Fatalf("contents has %d turns, want 1", len(contents))
	}
	parts := geminiArr(t, geminiObj(t, contents[0], "contents[0]")["parts"], "contents[0].parts")
	var images [][]byte
	var texts []string
	for i, p := range parts {
		part := geminiObj(t, p, "part")
		if raw, ok := part["inlineData"]; ok {
			inline := geminiObj(t, raw, "inlineData")
			if mime := geminiStr(t, inline["mimeType"], "mimeType"); mime != geminiImageMIME {
				t.Errorf("part %d mimeType = %q, want %q", i, mime, geminiImageMIME)
			}
			decoded, err := base64.StdEncoding.DecodeString(geminiStr(t, inline["data"], "data"))
			if err != nil {
				t.Fatalf("part %d inline data is not base64: %v", i, err)
			}
			images = append(images, decoded)
		}
		if text, ok := part["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	if len(images) != 1 {
		t.Fatalf("attached %d images, want exactly the one drawing", len(images))
	}
	if !bytes.Equal(images[0], req.Image) {
		t.Error("the inline image is not the drawing we were given")
	}
	if joined := strings.Join(texts, "\n"); !strings.Contains(joined, geminiTestPrompt) {
		t.Errorf("the prompt never reached the model; text parts were %q", joined)
	}
}

// A 200 whose payload violates the contract is a failure, not a score — and not
// worth a retry either, since temperature 0 buys the same answer twice.
func TestGeminiCritic_Critique_RejectsContractViolations(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"score above 1", `{"score":1.4,"feedback":"ok"}`},
		{"score below 0", `{"score":-0.2,"feedback":"ok"}`},
		{"a percentage instead of a fraction", `{"score":72,"feedback":"ok"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, stub := newGeminiTestCritic(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, geminiCritiqueEnvelope(tt.output))
			})
			got, err := c.Critique(context.Background(), geminiTestCritiqueRequest(t))
			if err == nil {
				t.Fatalf("expected a rejection, got critique %+v", got)
			}
			if !errors.Is(err, ErrInvalidCritique) {
				t.Errorf("error %v should wrap ErrInvalidCritique", err)
			}
			if got != (Critique{}) {
				t.Errorf("a rejected response must not leak a partial score, got %+v", got)
			}
			if n := stub.callCount(); n != 1 {
				t.Errorf("calls = %d, want 1 — a contract violation is not transient", n)
			}
		})
	}
}

// A 429 is the free tier's daily budget running out. It must be legible as exactly
// that, and must NOT be retried — the quota does not refill in 250ms.
func TestGeminiCritic_Critique_QuotaExhausted(t *testing.T) {
	const body = `{"error":{"code":429,"message":"You exceeded your current quota, please check your plan and billing details.","status":"RESOURCE_EXHAUSTED"}}`
	c, stub := newGeminiTestCritic(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusTooManyRequests, body)
	})

	_, err := c.Critique(context.Background(), geminiTestCritiqueRequest(t))
	if err == nil {
		t.Fatal("expected an error on 429")
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Errorf("error %v does not satisfy errors.Is(err, ErrQuotaExhausted)", err)
	}
	if n := stub.callCount(); n != 1 {
		t.Errorf("calls = %d, want exactly 1 — a daily quota cannot be retried away", n)
	}
}

// Over-long feedback is CLAMPED, not rejected, for the same reason the duel's
// reason is: the score is what decides anything, and it is validated strictly.
func TestGeminiCritic_Critique_ClampsOverlongFeedback(t *testing.T) {
	long := strings.Repeat("blah ", 200) + "final word"
	c, _ := newGeminiTestCritic(t, func(_ int, w http.ResponseWriter) {
		geminiWrite(w, http.StatusOK, geminiCritiqueEnvelope(`{"score":0.4,"feedback":"`+long+`"}`))
	})
	got, err := c.Critique(context.Background(), geminiTestCritiqueRequest(t))
	if err != nil {
		t.Fatalf("Critique: %v", err)
	}
	if n := utf8.RuneCountInString(got.Feedback); n > maxFeedbackLen {
		t.Errorf("feedback is %d runes, over the %d cap", n, maxFeedbackLen)
	}
	if !strings.HasSuffix(got.Feedback, "…") {
		t.Error("a clamped feedback should end in an ellipsis")
	}
	if got.Score != 0.4 {
		t.Errorf("clamping changed the score: %v", got.Score)
	}
}

// Bad input is caught before it costs a request: on a daily quota every wasted
// call is a drawing nobody gets scored.
func TestGeminiCritic_Critique_RejectsBadRequestWithoutCalling(t *testing.T) {
	good := pngCoverage(t, 8, 8, 0.5)
	tests := []struct {
		name string
		req  CritiqueRequest
	}{
		{"empty prompt", CritiqueRequest{Prompt: "  ", Image: good}},
		{"missing image", CritiqueRequest{Prompt: geminiTestPrompt}},
		{"the image is not a PNG", CritiqueRequest{Prompt: geminiTestPrompt, Image: []byte("not a png")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, stub := newGeminiTestCritic(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, geminiCritiqueEnvelope(`{"score":1,"feedback":"ok"}`))
			})
			if _, err := c.Critique(context.Background(), tt.req); err == nil {
				t.Fatal("expected a rejection before the request")
			}
			if n := stub.callCount(); n != 0 {
				t.Errorf("calls = %d, want 0 — bad input must not burn quota", n)
			}
		})
	}
}

// Everything the API can hand back that is not a critique must produce a clean
// error rather than a panic or a zero-value "0.0".
func TestGeminiCritic_Critique_MalformedResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantSaid string
	}{
		{"the text part is not JSON at all", geminiCritiqueEnvelope("I liked it, honestly."), "not the JSON critique"},
		{"no candidates because the request was blocked", `{"promptFeedback":{"blockReason":"SAFETY"}}`, "SAFETY"},
		{"the envelope itself is not JSON", `<!DOCTYPE html>502`, "decode response envelope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, stub := newGeminiTestCritic(t, func(_ int, w http.ResponseWriter) {
				geminiWrite(w, http.StatusOK, tt.body)
			})
			_, err := c.Critique(context.Background(), geminiTestCritiqueRequest(t))
			if err == nil {
				t.Fatal("expected an error, got a critique")
			}
			if !strings.Contains(err.Error(), tt.wantSaid) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantSaid)
			}
			// The critic names itself, so a log line says which of the two Gemini
			// callers failed — they have different fixes.
			if !strings.Contains(err.Error(), "gemini critic") {
				t.Errorf("error = %v, want it to identify the critic", err)
			}
			if n := stub.callCount(); n != 1 {
				t.Errorf("calls = %d, want 1 — a malformed 200 is not transient", n)
			}
		})
	}
}
