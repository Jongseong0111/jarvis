// Package usage 는 LLM 호출 비용을 JSONL 로 기록하고 집계한다.
package usage

// price 는 모델별 100만 토큰당 USD 단가다.
type price struct{ inPer1M, outPer1M float64 }

// geminiPrices 는 Gemini 모델별 가격표다(출처: ai.google.dev/gemini-api/docs/pricing, 2026-06).
var geminiPrices = map[string]price{
	"gemini-2.5-flash":      {inPer1M: 0.30, outPer1M: 2.50},
	"gemini-2.5-flash-lite": {inPer1M: 0.10, outPer1M: 0.40},
}

// geminiCost 는 모델·토큰 수로 USD 비용을 계산한다. 모르는 모델은 0(토큰은 별도 기록됨).
func geminiCost(model string, inTk, outTk int) float64 {
	p, ok := geminiPrices[model]
	if !ok {
		return 0
	}
	return p.inPer1M*float64(inTk)/1e6 + p.outPer1M*float64(outTk)/1e6
}
