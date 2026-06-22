package claudecode

import "testing"

func TestParseOutput_Cost(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"result","is_error":false,"result":"Hi","session_id":"s1","total_cost_usd":0.116938,"usage":{"input_tokens":5880,"output_tokens":5}}`)
	got, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if got.SessionID != "s1" || got.Text != "Hi" {
		t.Fatalf("base fields wrong: %+v", got)
	}
	if got.InputTk != 5880 || got.OutputTk != 5 || got.CostUSD != 0.116938 {
		t.Fatalf("cost fields wrong: %+v", got)
	}
}
