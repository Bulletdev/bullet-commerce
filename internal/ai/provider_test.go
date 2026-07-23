package ai

import (
	"context"
	"encoding/json"
)

// scriptedTurn is one modeled assistant turn the fakeProvider will "stream".
type scriptedTurn struct {
	text      string     // text the assistant emits this turn
	toolCalls []ToolCall // tool_use blocks (non-empty => stop_reason tool_use)
	err       error      // if set, Stream surfaces it via the stream's Err
}

// fakeProvider is an in-memory LLMProvider for tests — no network, no SDK types.
// It replays a script of turns; each Stream call consumes the next turn. It also
// records the requests it received so tests can assert what the agent sent.
type fakeProvider struct {
	turns    []scriptedTurn
	idx      int
	requests []LLMRequest
}

func (f *fakeProvider) Stream(_ context.Context, req LLMRequest) (LLMStream, error) {
	f.requests = append(f.requests, req)
	var turn scriptedTurn
	if f.idx < len(f.turns) {
		turn = f.turns[f.idx]
		f.idx++
	}

	events := make([]StreamEvent, 0, len(turn.toolCalls)+2)
	if turn.text != "" {
		events = append(events, StreamEvent{Kind: EventText, Text: turn.text})
	}
	stop := "end_turn"
	for i := range turn.toolCalls {
		tc := turn.toolCalls[i]
		events = append(events, StreamEvent{Kind: EventToolUse, ToolCall: &tc})
		stop = "tool_use"
	}
	events = append(events, StreamEvent{Kind: EventEnd, StopReason: stop})

	return &fakeStream{events: events, err: turn.err}, nil
}

type fakeStream struct {
	events []StreamEvent
	idx    int
	cur    StreamEvent
	err    error
}

func (s *fakeStream) Next() bool {
	if s.err != nil {
		return false
	}
	if s.idx >= len(s.events) {
		return false
	}
	s.cur = s.events[s.idx]
	s.idx++
	return true
}

func (s *fakeStream) Event() StreamEvent { return s.cur }
func (s *fakeStream) Err() error         { return s.err }
func (s *fakeStream) Close() error       { return nil }

// rawInput is a small helper to build tool-call input JSON in tests.
func rawInput(m map[string]any) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}
