package usage

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fixedRecorder(t *testing.T) (*Recorder, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "usage.jsonl") // 중간 디렉터리 자동 생성 확인
	r := NewRecorder(path)
	r.now = func() time.Time { return time.Date(2026, 6, 22, 14, 0, 0, 0, time.UTC) }
	return r, path
}

func readRecords(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	var recs []Record
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("unmarshal %q: %v", sc.Text(), err)
		}
		recs = append(recs, rec)
	}
	return recs
}

func TestRecorder_LogGemini(t *testing.T) {
	t.Parallel()
	r, path := fixedRecorder(t)
	r.LogGemini("agent", "gemini-2.5-flash", 2000, 100)
	recs := readRecords(t, path)
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	got := recs[0]
	want := Record{
		Ts: "2026-06-22T14:00:00Z", Source: "gemini", Feature: "agent",
		Model: "gemini-2.5-flash", InputTk: 2000, OutputTk: 100,
		CostUSD: 0.30*2000/1e6 + 2.50*100/1e6,
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRecorder_LogClaude(t *testing.T) {
	t.Parallel()
	r, path := fixedRecorder(t)
	r.LogClaude("kb", "claude", 5880, 5, 0.116938)
	recs := readRecords(t, path)
	if len(recs) != 1 || recs[0].Source != "claude" || recs[0].CostUSD != 0.116938 {
		t.Fatalf("unexpected: %+v", recs)
	}
}

func TestRecorder_ConcurrentAppend(t *testing.T) {
	t.Parallel()
	r, path := fixedRecorder(t)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.LogGemini("agent", "gemini-2.5-flash", 10, 1) }()
	}
	wg.Wait()
	if recs := readRecords(t, path); len(recs) != 50 {
		t.Fatalf("want 50 records, got %d", len(recs))
	}
}

func TestRecorder_BestEffortNoPanic(t *testing.T) {
	t.Parallel()
	// 디렉터리로 만들 수 없는 경로(부모가 파일) → 기록 실패해도 패닉/중단 없어야 함
	bad := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(bad, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(filepath.Join(bad, "usage.jsonl"))
	r.LogGemini("agent", "gemini-2.5-flash", 10, 1) // 패닉 없이 반환되면 통과
}

func TestFeatureContext(t *testing.T) {
	t.Parallel()
	if got := FeatureFromContext(context.Background()); got != "" {
		t.Fatalf("empty ctx want \"\", got %q", got)
	}
	ctx := WithFeature(context.Background(), "knowledge")
	if got := FeatureFromContext(ctx); got != "knowledge" {
		t.Fatalf("want knowledge, got %q", got)
	}
}
