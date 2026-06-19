# Dev Digest 아침 브리핑 설계

날짜: 2026-06-19  
상태: 확정

## 개요

매일 09:00에 개발 뉴스 3-5개(링크 포함)와 계층형 개발 공부 주제 3-5개를 Slack으로 전송한다.  
기존 Todoist 아침 브리핑(08:00)과 별개 메시지로, 같은 채널을 사용한다.

## 기능 범위

### 포함
- GeekNews(news.hada.io/rss) + HN(top stories API) 병렬 fetch → Gemini가 3-5개 선별·요약
- 도메인 로테이션 기반 계층형 공부 주제 3-5개 생성 (Gemini)
- 09:00 스케줄러 잡 (기존 인프로세스 스케줄러 활용)
- 추가 RSS URL 설정 지원 (`DIGEST_RSS_URLS`)

### 제외 (나중에)
- 웹 검색·트렌드 자동 탐색
- 특정 블로거/Naver 블로그 스크래핑

## 아키텍처

```
09:00 scheduler.Job
  ↓
agent.NewDevDigestBriefing(geminiClient, sender, channel)
  ↓
devdigest.Fetcher.Fetch()          — GeekNews RSS + HN API 병렬 fetch
devdigest.Generator.Generate()     — Gemini 1 call: 뉴스 선별 + 공부주제
  ↓
domain.MessageSender.Send()        — Slack 전송
```

## 파일 구조

```
internal/devdigest/
  fetcher.go        — RSS + HN API fetch (net/http, encoding/xml)
  fetcher_test.go   — httptest 기반
  digest.go         — Gemini 프롬프트 + 응답 파싱
  digest_test.go    — fake Gemini
internal/agent/
  devdigest_briefing.go       — NewDevDigestBriefing 스케줄러 잡
  devdigest_briefing_test.go
pkg/config/
  config.go         — DigestTime, DigestRSSURLs 추가
cmd/server/
  main.go           — 09:00 잡 등록
cmd/sendbrief/
  main.go           — -kind=digest 옵션 추가
```

## 설정

| 환경변수 | 기본값 | 설명 |
|---|---|---|
| `DIGEST_TIME` | `09:00` | 전송 시각 (HH:MM, Asia/Seoul) |
| `DIGEST_RSS_URLS` | `` | 쉼표 구분 추가 RSS URL (선택) |

- `TODOIST_BRIEFING_CHANNEL` 재사용 (새 채널 환경변수 없음)
- `TODOIST_BRIEFING_TZ` 재사용
- `GEMINI_API_KEY` / `GEMINI_MODEL` 재사용

GeekNews RSS(`https://news.hada.io/rss`)와 HN(`https://hacker-news.firebaseio.com/v0`)은 내장 기본값.

## 뉴스 Fetch 방식

### GeekNews
- `GET https://news.hada.io/rss` → XML 파싱 (RSS 2.0)
- 최신 30개 `<item>` 수집: title, link, description

### Hacker News
- `GET /v0/topstories.json` → ID 배열
- 상위 30개 ID에 대해 `/v0/item/{id}.json` 병렬 fetch (goroutine + WaitGroup)
- score, type=="story", url 있는 것만 사용

### 합산
- 두 소스 합쳐 최대 60개 후보 → Gemini에 전달
- `DIGEST_RSS_URLS`의 추가 피드도 동일하게 RSS 파싱

## Gemini 프롬프트 구조

단일 호출로 뉴스 선별 + 공부주제를 동시에 생성한다.

```
너는 개발자를 위한 아침 다이제스트를 만드는 어시스턴트다.

[뉴스 후보 목록]
1. 제목 | URL | 설명
2. ...

[작업]
1. 위 목록에서 개발자에게 가장 흥미로운 항목 3-5개를 골라라.
   - 실제 기술 내용이 있는 것 우선 (채용/마케팅 제외)
   - 각 항목: 제목(원문 유지), URL, 한국어 한줄 요약
   
2. 오늘의 개발 공부 주제를 생성하라.
   - 아래 도메인 중 하나를 선택: 언어 / 웹·백엔드 / 데이터베이스 / 인프라 / 데이터 / 운영체제 / 네트워크 / 자료구조·알고리즘 / 개발도구 / AI / 기타
   - 인프라 선택 시 Kafka·RabbitMQ 같은 메시징 시스템도 포함 가능
   - 선택한 도메인 아래로 계층형 주제 3-5개 생성
   - 형식: 도메인 → 중분류 → 구체 개념 (예: 데이터베이스 → Vector DB → HNSW 인덱스 구조)

JSON으로 응답:
{
  "news": [{"title": "...", "url": "...", "summary": "..."}],
  "domain": "...",
  "topics": ["도메인 → ... → ...", ...]
}
```

## Slack 메시지 포맷

```
📰 *오늘의 개발 소식*
• <https://...|제목> — 한줄 요약
• <https://...|제목> — 한줄 요약
• ...

📚 *오늘의 공부 주제*  _(도메인: 데이터베이스)_
• 데이터베이스 → Vector DB → HNSW 인덱스 구조
• 데이터베이스 → PostgreSQL → MVCC 동시성 제어
• ...
```

## 에러 처리

- Fetch 실패(네트워크 오류 등): 성공한 소스만으로 진행, 0개면 공부주제만 전송
- Gemini 실패: 로그 남기고 브리핑 건너뜀 (무음 실패)
- Slack 전송 실패: 기존 `sendText` 패턴과 동일

## 테스트 전략

- `fetcher_test.go`: `httptest.Server`로 RSS·HN API 모킹
- `digest_test.go`: fake Gemini 클라이언트로 JSON 파싱 검증
- `devdigest_briefing_test.go`: fake fetcher + fake generator로 전송 포맷 검증
- 스케줄러 통합은 기존 `scheduler_test.go` 패턴 재사용

## sendbrief 확장

```bash
go run ./cmd/sendbrief -kind=digest   # 즉시 전송 테스트
```

## 도메인 목록 (하드코딩)

```go
var domains = []string{
    "언어", "웹·백엔드", "데이터베이스", "인프라",
    "데이터", "운영체제", "네트워크",
    "자료구조·알고리즘", "개발도구", "AI", "기타",
}
```
