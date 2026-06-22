package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"time"
)

// Bucket 은 한 키(model 또는 feature)의 집계다.
type Bucket struct {
	Key      string
	Calls    int
	InputTk  int
	OutputTk int
	CostUSD  float64
}

// Summary 는 기간 집계 결과다.
type Summary struct {
	From, To   time.Time
	TotalCost  float64
	TotalCalls int
	ByModel    []Bucket
	ByFeature  []Bucket
}

// Query 는 [from, to) 구간의 레코드를 읽어 집계한다. 파일 없으면 빈 Summary.
func (r *Recorder) Query(from, to time.Time) (Summary, error) {
	s := Summary{From: from, To: to}
	r.mu.Lock()
	data, err := os.ReadFile(r.path)
	r.mu.Unlock()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}

	models := map[string]*Bucket{}
	feats := map[string]*Bucket{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue // 깨진 줄은 건너뜀(best-effort)
		}
		ts, err := time.Parse(time.RFC3339, rec.Ts)
		if err != nil {
			continue
		}
		if ts.Before(from) || !ts.Before(to) { // [from, to)
			continue
		}
		s.TotalCalls++
		s.TotalCost += rec.CostUSD
		addBucket(models, rec.Model, rec)
		addBucket(feats, rec.Feature, rec)
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	s.ByModel = sortedBuckets(models)
	s.ByFeature = sortedBuckets(feats)
	return s, nil
}

func addBucket(m map[string]*Bucket, key string, rec Record) {
	b := m[key]
	if b == nil {
		b = &Bucket{Key: key}
		m[key] = b
	}
	b.Calls++
	b.InputTk += rec.InputTk
	b.OutputTk += rec.OutputTk
	b.CostUSD += rec.CostUSD
}

func sortedBuckets(m map[string]*Bucket) []Bucket {
	out := make([]Bucket, 0, len(m))
	for _, b := range m {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostUSD != out[j].CostUSD {
			return out[i].CostUSD > out[j].CostUSD
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// RangeForPeriod 는 period("today"|"week"|"month")에 대한 [from, to) 를 now 기준으로 계산한다.
// to 는 now(미래 레코드까지 포함하도록 넉넉히), from 은 구간 시작 00:00.
func RangeForPeriod(now time.Time, period string) (from, to time.Time) {
	loc := now.Location()
	y, mo, d := now.Date()
	startOfDay := time.Date(y, mo, d, 0, 0, 0, 0, loc)
	switch period {
	case "week":
		// 월요일 시작
		offset := (int(now.Weekday()) + 6) % 7
		from = startOfDay.AddDate(0, 0, -offset)
	case "month":
		from = time.Date(y, mo, 1, 0, 0, 0, 0, loc)
	default: // today
		from = startOfDay
	}
	return from, now
}
