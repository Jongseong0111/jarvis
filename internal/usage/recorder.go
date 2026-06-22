package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record 는 usage.jsonl 한 줄(= LLM API 호출 1건)이다.
type Record struct {
	Ts       string  `json:"ts"`
	Source   string  `json:"source"`  // "gemini" | "claude"
	Feature  string  `json:"feature"` // agent|vision|knowledge|digest|kb
	Model    string  `json:"model"`
	InputTk  int     `json:"input_tk"`
	OutputTk int     `json:"output_tk"`
	CostUSD  float64 `json:"cost_usd"`
}

// Recorder 는 LLM 호출 비용을 append-only JSONL 로 기록한다(동시 쓰기 안전).
type Recorder struct {
	path string
	mu   sync.Mutex
	now  func() time.Time
}

// NewRecorder 는 path(JSONL 파일)에 기록하는 Recorder 를 만든다.
func NewRecorder(path string) *Recorder {
	return &Recorder{path: path, now: time.Now}
}

// LogGemini 는 Gemini 호출 1건을 기록한다(가격표로 cost 계산).
func (r *Recorder) LogGemini(feature, model string, inputTk, outputTk int) {
	r.append(Record{
		Source: "gemini", Feature: feature, Model: model,
		InputTk: inputTk, OutputTk: outputTk, CostUSD: geminiCost(model, inputTk, outputTk),
	})
}

// LogClaude 는 Claude 호출 1건을 기록한다(cost 는 CLI 가 직접 제공).
func (r *Recorder) LogClaude(feature, model string, inputTk, outputTk int, costUSD float64) {
	r.append(Record{
		Source: "claude", Feature: feature, Model: model,
		InputTk: inputTk, OutputTk: outputTk, CostUSD: costUSD,
	})
}

// append 는 Record 를 JSONL 한 줄로 파일 끝에 붙인다. best-effort(실패는 무시).
func (r *Recorder) append(rec Record) {
	rec.Ts = r.now().Format(time.RFC3339)
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
