package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type reasoningLoopSample struct {
	Name      string `json:"name"`
	Reasoning string `json:"reasoning"`
}

type reasoningLoopCorpus struct {
	Loops  []reasoningLoopSample `json:"loops"`
	Normal []reasoningLoopSample `json:"normal"`
}

func loadReasoningLoopCorpus(t *testing.T) reasoningLoopCorpus {
	t.Helper()
	data, err := os.ReadFile("testdata/reasoning_loops.json")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var corpus reasoningLoopCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("decode corpus: %v", err)
	}
	if len(corpus.Loops) != 3 {
		t.Fatalf("loops = %d, want 3", len(corpus.Loops))
	}
	return corpus
}

func detectLoopAt(t *testing.T, reasoning string) (int, *reasoningLoopDetector) {
	t.Helper()
	detector := newReasoningLoopDetector()
	for cut := 250; cut <= len(reasoning); cut += 250 {
		if detector.Observe(reasoning[:cut]) {
			return cut, detector
		}
	}
	if detector.Observe(reasoning) {
		return len(reasoning), detector
	}
	return 0, detector
}

func TestReasoningLoopDetectorRealSamples(t *testing.T) {
	corpus := loadReasoningLoopCorpus(t)
	for _, sample := range corpus.Loops {
		cut, detector := detectLoopAt(t, sample.Reasoning)
		if cut == 0 {
			t.Fatalf("%s: loop not detected (chars=%d)", sample.Name, len(sample.Reasoning))
		}
		t.Logf("LOOP %s chars=%d cut_at_bytes=%d cut_chars=%d block=%d repeats=%d",
			sample.Name, len(sample.Reasoning), cut, detector.cutChars, detector.blockChars, detector.repeats)
		if detector.blockChars < 150 || detector.repeats < 5 {
			t.Fatalf("%s: weak detection block=%d repeats=%d", sample.Name, detector.blockChars, detector.repeats)
		}
	}
	// The fast draft loop must be cut well before the end.
	if cut, _ := detectLoopAt(t, corpus.Loops[0].Reasoning); cut > 6000 {
		t.Fatalf("first loop cut at %d chars, want <= 6000", cut)
	}
}

func TestReasoningLoopDetectorNormalSamples(t *testing.T) {
	corpus := loadReasoningLoopCorpus(t)
	for _, sample := range corpus.Normal {
		if cut, detector := detectLoopAt(t, sample.Reasoning); cut != 0 {
			t.Fatalf("%s: false positive at %d chars (block=%d repeats=%d)",
				sample.Name, cut, detector.blockChars, detector.repeats)
		}
	}
}

func TestRecoveryHelpers(t *testing.T) {
	if !shouldRecoverEmptyLength(&providers.LLMResponse{FinishReason: "length"}) {
		t.Fatal("length + empty must be recoverable")
	}
	if !shouldRecoverEmptyLength(&providers.LLMResponse{FinishReason: "max_tokens"}) {
		t.Fatal("max_tokens + empty must be recoverable")
	}
	if shouldRecoverEmptyLength(&providers.LLMResponse{Content: "answer", FinishReason: "length"}) {
		t.Fatal("non-empty response must not be retried")
	}
	if shouldRecoverEmptyLength(&providers.LLMResponse{FinishReason: "stop"}) {
		t.Fatal("stop + empty must not be retried as a length issue")
	}

	messages := []providers.Message{{Role: "user", Content: "hello"}}
	withNote := withRecoveryNote(messages)
	if len(withNote) != 2 || withNote[1].Role != "user" || withNote[1].Content != recoveryRetryNote {
		t.Fatalf("recovery note = %#v", withNote)
	}
	if len(messages) != 1 {
		t.Fatalf("original messages mutated: %#v", messages)
	}
}

func TestReasoningLoopCutRetriesOnceWithNote(t *testing.T) {
	cfg := newConfiguredStreamingTestConfig(t, true, true, nil)
	streamer := &recordingStreamer{}
	msgBus := bus.NewMessageBus()
	msgBus.SetStreamDelegate(configuredStreamingDelegate{streamer: streamer})

	block := strings.Repeat(
		"This is a long planning paragraph that the model keeps repeating instead of answering. ",
		3,
	)
	chunks := make([]providers.StreamChunk, 0, 14)
	acc := ""
	for i := 0; i < 14; i++ {
		acc += block + "\n"
		chunks = append(chunks, providers.StreamChunk{ReasoningContent: acc})
	}

	provider := &configuredStreamingProvider{
		eventPlan: []configuredStreamingEventCall{
			{chunks: chunks},
			{
				chunks:   []providers.StreamChunk{{Content: "recovered"}},
				response: &providers.LLMResponse{Content: "recovered", FinishReason: "stop"},
			},
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)
	got := runConfiguredStreamingTurn(t, al, "pico")
	if got != "recovered" {
		t.Fatalf("response = %q, want recovered", got)
	}
	if provider.eventCalls != 2 {
		t.Fatalf("ChatStreamEvents calls = %d, want 2", provider.eventCalls)
	}
	last := provider.eventMessages[1]
	if len(last) == 0 || last[len(last)-1].Content != recoveryRetryNote {
		t.Fatalf("retry note missing from second call: %#v", last)
	}
}

func TestReasoningLoopCutGivesClearMessageAfterOneRetry(t *testing.T) {
	cfg := newConfiguredStreamingTestConfig(t, true, true, nil)
	streamer := &recordingStreamer{}
	msgBus := bus.NewMessageBus()
	msgBus.SetStreamDelegate(configuredStreamingDelegate{streamer: streamer})

	block := strings.Repeat(
		"This is a long planning paragraph that the model keeps repeating instead of answering. ",
		3,
	)
	buildChunks := func() []providers.StreamChunk {
		chunks := make([]providers.StreamChunk, 0, 14)
		acc := ""
		for i := 0; i < 14; i++ {
			acc += block + "\n"
			chunks = append(chunks, providers.StreamChunk{ReasoningContent: acc})
		}
		return chunks
	}
	provider := &configuredStreamingProvider{
		eventPlan: []configuredStreamingEventCall{
			{chunks: buildChunks()},
			{chunks: buildChunks()},
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)
	got := runConfiguredStreamingTurn(t, al, "pico")
	if got != reasoningLoopResponse {
		t.Fatalf("response = %q, want clear loop message", got)
	}
	if provider.eventCalls != 2 {
		t.Fatalf("ChatStreamEvents calls = %d, want 2", provider.eventCalls)
	}
}

type recoveryCaptureProvider struct {
	responses []*providers.LLMResponse
	messages  [][]providers.Message
	calls     int
}

func (p *recoveryCaptureProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	p.messages = append(p.messages, append([]providers.Message(nil), messages...))
	idx := p.calls
	p.calls++
	if idx < len(p.responses) {
		return p.responses[idx], nil
	}
	return &providers.LLMResponse{Content: "done", FinishReason: "stop"}, nil
}

func (p *recoveryCaptureProvider) GetDefaultModel() string { return "recovery-model" }

func TestEmptyLengthResponseRetriesOnce(t *testing.T) {
	provider := &recoveryCaptureProvider{responses: []*providers.LLMResponse{
		{Content: "", FinishReason: "length"},
		{Content: "recovered", FinishReason: "stop"},
	}}
	al, agent, cleanup := newTurnCoordTestLoop(t, provider)
	defer cleanup()

	pipeline := NewPipeline(al)
	ts := newTurnState(agent, makeTestProcessOpts("recovery-session"), turnEventScope{
		turnID:  "turn-1",
		context: newTurnContext(nil, nil, nil),
	})
	exec, err := pipeline.SetupTurn(context.Background(), ts)
	if err != nil {
		t.Fatalf("SetupTurn failed: %v", err)
	}
	ctrl, err := pipeline.CallLLM(context.Background(), context.Background(), ts, exec, 1)
	if err != nil {
		t.Fatalf("CallLLM failed: %v", err)
	}
	if ctrl != ControlBreak || exec.finalContent != "recovered" {
		t.Fatalf("ctrl=%v final=%q", ctrl, exec.finalContent)
	}
	if provider.calls != 2 {
		t.Fatalf("calls = %d, want 2", provider.calls)
	}
	last := provider.messages[1]
	if len(last) == 0 || last[len(last)-1].Content != recoveryRetryNote {
		t.Fatalf("retry note missing: %#v", last)
	}
}

func TestEmptyLengthResponseGivesClearMessageAfterOneRetry(t *testing.T) {
	provider := &recoveryCaptureProvider{responses: []*providers.LLMResponse{
		{Content: "", FinishReason: "length"},
		{Content: "", FinishReason: "length"},
	}}
	al, agent, cleanup := newTurnCoordTestLoop(t, provider)
	defer cleanup()

	pipeline := NewPipeline(al)
	ts := newTurnState(agent, makeTestProcessOpts("recovery-session"), turnEventScope{
		turnID:  "turn-1",
		context: newTurnContext(nil, nil, nil),
	})
	exec, err := pipeline.SetupTurn(context.Background(), ts)
	if err != nil {
		t.Fatalf("SetupTurn failed: %v", err)
	}
	ctrl, err := pipeline.CallLLM(context.Background(), context.Background(), ts, exec, 1)
	if err != nil {
		t.Fatalf("CallLLM failed: %v", err)
	}
	if ctrl != ControlBreak || exec.finalContent != emptyLengthResponse {
		t.Fatalf("ctrl=%v final=%q, want clear message", ctrl, exec.finalContent)
	}
	if provider.calls != 2 {
		t.Fatalf("calls = %d, want 2 (one retry only)", provider.calls)
	}
}
