# CLAUDE.md

이 레포는 개인 자동화 에이전트 서버다.

사용자는 Slack, 추후 카카오톡/음성/웹 UI 등 하나의 입력 채널로 자연어 명령을 보낸다.
서버는 입력을 받아 의도를 판단하고, 적절한 기능 모듈을 실행한 뒤, 결과를 다시 입력 채널로 응답한다.

목표는 “사용자가 자연어로 말하면 집 정리 시스템, 지식 저장소, 할일 관리 등을 대신 조회/수정/정리해주는 개인 운영 에이전트”를 만드는 것이다.

## 핵심 목표

* 입력 채널은 하나로 통합한다.
* 기능은 모듈 단위로 확장한다.
* LLM은 의도 판단과 작업 계획 수립에 사용한다.
* 실제 외부 시스템 수정은 제한된 Tool/Worker 코드로 수행한다.
* 중요한 변경은 바로 적용하지 않고 리뷰/승인 플로우를 거친다.
* 모든 작업 결과는 사용자에게 요약해서 돌려준다.

## 전체 구조

```txt
Input Channel
  - Slack
  - later: KakaoTalk
  - later: Web UI
  - later: Voice

        ↓

Agent Server

        ↓

Intent Router

        ↓

Workers
  - HomeOrganizerWorker
  - KnowledgeRepoWorker
  - TodoWorker
  - SchedulerWorker

        ↓

External Systems
  - Notion
  - Git repository
  - Claude Code
  - GitHub
```

## 1차 목표

처음에는 Slack 입력만 지원한다.

```txt
Slack message
  ↓
Local Mac server
  ↓
Intent Router
  ↓
Worker execution
  ↓
Slack reply
```

서버는 사용자의 MacBook 또는 데스크톱에서 실행될 수 있다.
처음에는 클라우드 서버를 전제로 하지 않는다.

## 주요 기능

### 1. 집 정리 시스템

사용자가 자연어로 물품 위치를 추가/수정/삭제/검색할 수 있어야 한다.

예시 입력:

```txt
아기방 서랍장 두번째 칸에 손수건 10개 넣었어
건전지 어디 뒀지?
AAA 건전지 2개 썼어
거실 TV장 아래 서랍에 리모컨 배터리 넣어둔 거 삭제해줘
```

저장소는 Notion DB를 사용한다.

집 정리 Worker는 다음 기능을 제공한다.

* 물품 추가
* 물품 위치 수정
* 수량 수정
* 물품 검색
* 물품 삭제 후보 생성
* 변경안 리뷰 요청
* 승인 후 Notion 반영

집 정리 시스템에서는 LLM이 Notion을 자유롭게 수정하면 안 된다.
LLM은 자연어를 해석하고 변경안을 만드는 데만 사용한다.
실제 Notion 수정은 서버 코드의 제한된 함수로만 수행한다.

### 2. 지식 저장소 시스템

사용자가 개발 개념, 에러, 운영 경험, 공부 내용을 자연어로 입력하면 별도 Git 지식 저장소에 정리한다.

예시 입력:

```txt
TLS랑 인증서 개념 정리해서 지식 저장소 업데이트해줘
ECS Task Role이랑 Execution Role 차이 정리해줘
오늘 배운 Go heap 인터페이스 내용 저장해줘
CloudWatch 로그그룹 안 보였던 문제 트러블슈팅으로 정리해줘
```

지식 저장소는 별도 Git repo로 관리한다.

KnowledgeRepoWorker는 다음 방식으로 동작한다.

```txt
1. 사용자 요청 수신
2. 지식 저장소 repo 경로로 이동
3. Claude Code headless 실행
4. Claude Code가 기존 문서를 읽고 수정/생성
5. git diff 수집
6. 사용자에게 변경 요약과 diff 리뷰 요청
7. 승인하면 commit
8. 필요하면 push
```

지식 저장소는 Git diff로 리뷰 가능하므로 Claude Code를 활용해도 된다.
다만 승인 전에는 commit/push하지 않는다.

### 3. Todo 시스템

추후 Notion Todo DB 또는 별도 저장소와 연동한다.

예시 입력:

```txt
오늘 할일에 Clone Graph 다시 풀기 추가해줘
오늘 할일 뭐야?
이번주에 집 정리 사진 찍기 넣어줘
```

초기 구현 대상은 아니다.

### 4. 스케줄러

추후 매일/매주 자동 실행 기능을 지원한다.

예시:

```txt
매일 아침 오늘 할일 요약
매주 일요일 집 정리 미완료 항목 요약
매일 밤 공부 기록 정리 요청
```

초기 구현 대상은 아니다.

## Intent Router

입력 메시지를 보고 어떤 Worker가 처리할지 판단한다.

가능한 intent 예시:

```txt
home.search
home.add
home.update
home.delete

knowledge.update
knowledge.search
knowledge.review

todo.add
todo.list
todo.update

system.help
system.unknown
```

Router는 처음부터 완벽할 필요 없다.
초기에는 LLM을 사용해 intent를 분류하되, 결과는 엄격한 enum으로 제한한다.

잘 모르겠으면 `system.unknown`으로 처리하고 사용자에게 되묻는다.

## 리뷰/승인 플로우

중요한 변경은 바로 적용하지 않는다.

### 집 정리

Notion 변경 전 Slack에 다음 형태로 보여준다.

```txt
변경안:

작업: 물품 추가
품목: 손수건
구역: 아기방
위치: 서랍장
상세위치: 두번째 칸
수량: 10개

적용할까?
[승인] [수정] [취소]
```

승인 후에만 Notion DB에 반영한다.

### 지식 저장소

Claude Code 실행 후 바로 commit하지 않는다.

먼저 Slack에 요약한다.

```txt
지식 저장소 변경안:

수정 파일:
- concepts/network/tls.md
- concepts/network/certificate.md

요약:
- TLS 핸드셰이크 설명 추가
- 인증서와 CA 역할 정리
- HTTPS 연결 흐름 예시 추가

적용할까?
[승인] [수정요청] [취소]
```

승인하면 commit한다.

## 안전 원칙

* 삭제는 항상 승인 필요.
* 대량 변경은 항상 승인 필요.
* Notion DB schema 변경은 자동으로 하지 않는다.
* Git repo에서 승인 전 commit/push 금지.
* 외부 시스템 API key는 코드에 하드코딩하지 않는다.
* `.env` 또는 OS keychain을 사용한다.
* 모든 작업은 로그로 남긴다.
* 실패 시 사용자에게 실패 사유를 짧게 알려준다.

## 권장 디렉터리 구조

```txt
personal-agent-server/
  cmd/
    server/
      main.go

  internal/
    app/
      app.go

    slack/
      client.go
      handler.go

    router/
      intent_router.go
      types.go

    workers/
      home/
        worker.go
        notion_client.go
        types.go

      knowledge/
        worker.go
        claude_runner.go
        git.go
        types.go

      todo/
        worker.go

    approval/
      store.go
      types.go

    llm/
      client.go
      prompts.go

    config/
      config.go

    logger/
      logger.go

  docs/
    architecture.md
    setup.md

  .env.example
  README.md
  CLAUDE.md
```

## 구현 우선순위

### Phase 1: Slack 입력 수신

* Slack Bot 생성
* Slack 메시지 수신
* 서버에서 메시지 로그 출력
* Slack으로 echo 응답

### Phase 2: Intent Router

* 메시지를 intent로 분류
* `home.*`, `knowledge.*`, `system.unknown` 정도만 먼저 지원
* Worker 호출 구조 만들기

### Phase 3: 집 정리 Notion 연동

* Notion DB 설정
* 물품 추가 변경안 생성
* 승인 플로우
* 승인 시 Notion 반영
* 물품 검색

### Phase 4: 지식 저장소 Claude Code 연동

* 지식 저장소 repo path 설정
* Claude Code headless 실행
* git diff 수집
* Slack 리뷰 요청
* 승인 시 commit

### Phase 5: 확장

* Todo
* 스케줄러
* Web UI
* 카카오톡
* 음성 입력

## Claude Code 실행 방식

KnowledgeRepoWorker는 로컬에 설치된 Claude Code CLI를 실행할 수 있다.

예시:

```bash
claude -p "사용자 요청을 바탕으로 이 지식 저장소를 업데이트해줘. 기존 문서를 먼저 확인하고 필요한 파일만 수정해."
```

Go 코드에서는 subprocess로 실행한다.

```go
cmd := exec.Command("claude", "-p", prompt)
cmd.Dir = knowledgeRepoPath
out, err := cmd.CombinedOutput()
```

Claude Code 실행 후 반드시 git diff를 확인한다.

```bash
git diff --stat
git diff
```

## 지식 저장소 Worker 규칙

KnowledgeRepoWorker는 Claude Code에게 다음 규칙을 전달해야 한다.

```txt
너는 개인 개발 지식 저장소를 업데이트하는 에이전트다.

규칙:
- 기존 문서를 먼저 검색한다.
- 관련 문서가 있으면 새 문서를 만들지 말고 보강한다.
- 관련 문서가 없으면 적절한 위치에 새 문서를 만든다.
- 문서는 한국어로 작성한다.
- 파일명은 영어 kebab-case를 사용한다.
- 사용자의 실무 맥락을 살려 정리한다.
- 불확실한 내용은 확인 필요로 표시한다.
- 작업 후 수정/생성한 파일 목록과 요약을 출력한다.
```

## 집 정리 Worker 규칙

HomeOrganizerWorker는 Claude Code를 쓰지 않고 일반 LLM + Notion API를 사용한다.

이유:

* 집 정리는 구조화 데이터다.
* Notion DB schema를 보호해야 한다.
* 자유 수정보다 제한된 함수 호출이 안전하다.

LLM은 다음만 수행한다.

```txt
사용자 메시지를 읽고 아래 중 하나의 변경안으로 변환한다.

- 물품 추가
- 물품 검색
- 물품 수정
- 물품 삭제 요청
- 알 수 없음
```

실제 Notion 수정은 서버 코드가 수행한다.

## 환경변수 예시

```env
SLACK_BOT_TOKEN=
SLACK_APP_TOKEN=
SLACK_SIGNING_SECRET=

OPENAI_API_KEY=
ANTHROPIC_API_KEY=

NOTION_API_KEY=
NOTION_HOME_DB_ID=

KNOWLEDGE_REPO_PATH=/Users/me/repos/personal-knowledge

GIT_AUTHOR_NAME=
GIT_AUTHOR_EMAIL=
```

## 응답 스타일

Slack 응답은 짧고 명확하게 한다.

좋은 예:

```txt
건전지 위치를 찾았어.

AA 건전지: 거실 TV장 아래 서랍, 8개
AAA 건전지: 작업방 책상 오른쪽 서랍, 4개
```

좋지 않은 예:

```txt
사용자의 요청을 분석한 결과 데이터베이스에서 여러 항목이 발견되었으며...
```

## 최종 방향

이 서버는 단순 챗봇이 아니다.

사용자의 자연어 입력을 받아 여러 개인 시스템을 대신 조작하는 개인 운영 에이전트다.

처음에는 작게 시작한다.

```txt
Slack
→ Agent Server
→ Notion 집 정리
→ Git 지식 저장소
```

이후 점진적으로 확장한다.
