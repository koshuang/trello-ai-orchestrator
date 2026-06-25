# Trello AI Orchestrator — Agent Guide

## Overview

Trello AI Orchestrator 是一個 Go 服務，接收 Trello webhook 事件（commentCard），根據 @mention 及「固定命令 / keyword / AI fallback」三層決策引擎決定行為，並在 LLM API key 未設定時以 stub 模式運作。採用 Clean Architecture 分層。

### Tech Stack

- **語言**: Go 1.25+（[go.mod](/go.mod) 指定 `go 1.25.0`）
- **HTTP 框架**: Gin（adapter/controller/）
- **資料庫**: SQLite（via mattn/go-sqlite3, infrastructure/database/）
- **測試**: stretchr/testify（assert, require）
- **無外部依賴的 AI 決策引擎**: 內建 stub（LLM_API_KEY 空值時自動啟用）

### 目錄結構

```
├── main.go                    # Entry point — DI 組裝
├── config/
│   └── config.go              # Config struct + LoadConfig()
├── domain/                    # 純資料實體，零依賴
│   ├── event.go               # ProcessedEvent
│   ├── github.go              # GitHubIssuePayload / GitHubIssueResponse
│   ├── llm.go                 # LLMInput / LLMResponse / LLMStateUpdate / LLMGitHubIssue / LLMPlan
│   ├── state.go               # WorkflowState + 狀態常數
│   └── trello.go              # TrelloWebhookPayload / TrelloCardContext / TrelloAIStateBlock ...
├── usecase/                   # 商業邏輯
│   ├── interfaces.go          # 所有 repository / gateway 介面
│   ├── webhook_interactor.go  # 核心流程 ProcessEvent()
│   ├── state_helper.go        # FormatAIState / UpdateDescriptionWithAIState / ParseAIState
│   ├── webhook_interactor_test.go  # 既有 unit tests（mocks 定義在此）
│   └── scenario_test.go       # 15 個情境測試（新）
├── adapter/
│   ├── controller/
│   │   ├── webhook_handler.go      # Gin handler（POST /webhooks/trello）
│   │   ├── health_handler.go       # /health + /workflow-states/:cardId
│   │   └── trello_integration_test.go  # 7 個 HTTP 整合測試（新）
│   ├── gateway/
│   │   ├── llm_client.go           # LLM 客戶端 + stub 決策引擎
│   │   ├── trello_client.go        # Trello API 客戶端
│   │   └── github_client.go        # GitHub API 客戶端
│   └── repository/
│       └── sqlite_repo.go          # SQLite CRUD
└── infrastructure/
    └── database/
        └── sqlite.go               # SQLite 初始化 + schema migration
```

## LLM 決策引擎

### 三層路由（adapter/gateway/llm_client.go: `decideStub`）

有 `LLM_API_KEY` 時走真實 LLM API（Gemini / Anthropic）；無設定時自動降級為 stub：

1. **固定命令**（最高優先）— 強開頭比對 `stripMentions` 後的 cleaned text：
   - `!issue <title>` → `create_github_issue`
   - `!plan <desc>` → `create_plan`
   - `!run <script>` → `reply_comment`（含 script name）
   - `!reply <text>` → `reply_comment`（含自訂回覆）
2. **Keyword 比對** — `create issue` / `github issue` / `create plan` / `implementation plan` / `run claude` / `run codex` / `reply` / `clarify` / `ignore`
3. **AI fallback** — 回 `update_state_only`（標記需 LLM 分析）

### @mention 過濾（usecase/webhook_interactor.go）

@mention 過濾在 **interactor 層**，不在 stub 中。`containsMention(comment, botIDs)` 比對大小寫不敏感。

```go
if len(w.cfg.BotTrelloMemberIDs) > 0 && !containsMention(commentText, w.cfg.BotTrelloMemberIDs) {
    // 記錄 success event 但跳過 LLM 處理
    return nil
}
```

### 參數萃取

`extractArg(comment, prefix, fallback)` — 在 comment 中搜尋 prefix 後綴文字（`!issue Fix login` → `"Fix login"`）。

## Workflow 流程

```
Trello webhook POST
  → Gin router (controller/webhook_handler.go)
    → HandlePost() 解析 JSON、驗證簽名（可選）
      → WebhookInteractor.ProcessEvent()
        1. Bot ID 過濾
        2. Idempotency 檢查（事件 ID 去重）
        3. 載入 / 建立 WorkflowState（SQLite）
        4. FetchCardContext() 取得完整卡片資料
        5. @mention 過濾
        6. LLM Decide() → 取得決策
        7. 執行 Action（reply / create issue / create plan / update state）
        8. 同步 AI State 到 Trello 卡片描述
        9. 儲存 WorkflowState
        10. 標記事件為已處理
```

## 狀態機（domain/state.go）

```
new → needs_triage → needs_pm_clarification ─┐
                    → ready_for_issue ────────┼──→ ready_for_implementation → implementation_in_progress → waiting_for_review → done
                    → issue_created ──────────┘
                    → plan_created ───────────┘
                    → ignored / error（終端）
```

## 測試

### 情境測試（15 個，readable harness）

檔案：`usecase/scenario_test.go`

```go
// 用法
s := newScenario(t, withConfig(func(c *config.Config) {
    c.BotTrelloMemberIDs = []string{"my-bot"}
}))
payload := makeComment("user_1", "john", "@my-bot !issue Fix bug", "card_001", "Bug", "bg001")
err := s.exec(t, payload)
assert.Equal(t, "issue_created", string(s.stateRepo.saved.Status))
```

測試涵蓋：@mention 忽略、@mention 觸發、bot 忽略、重複事件、`!issue` / `!plan` / `!run` / `!reply` 固定命令、Safe Mode 不寫入、duplicate issue 防護、AI keyword routing、unsupported action type、新卡片 / 既有卡片、LLM 錯誤、多則 comments。

### HTTP 整合測試（7 個）

檔案：`adapter/controller/trello_integration_test.go`

啟動真實 Gin server（httptest.NewServer）發送 HTTP POST，驗證 response status + mock side-effects。涵蓋：無 @mention、@mention 觸發、`!issue` 命令、bot 忽略、unsupported type、safe mode、invalid JSON。

### 既有 Unit Tests（7 個）

檔案：`usecase/webhook_interactor_test.go`

使用同一套 mock 型別進行隔離測試。

### 執行方式

```bash
make test              # 全部
make test-scenario     # 15 情境測試
make test-http         # 7 HTTP 測試
make test-unit         # 既有 unit tests
go test -v -run "Scenario" ./usecase/ -count=1
go test -v -run "Integration" ./adapter/controller/ -count=1
```

### Mock 型別

定義在 `usecase/webhook_interactor_test.go`，所有 test 檔案共用：

- `mockStateRepo` — `GetByCardID()` / `Save()`
- `mockEventRepo` — `Exists()` / `Create()`
- `mockTrelloGateway` — `FetchCardContext()` / `AddComment()` / `UpdateCardDescription()`
- `mockGitHubGateway` — `CreateIssue()` / `UpdateIssue()`
- `mockLLMGateway` — 回應或錯誤可預先設定

## 環境變數

| 變數 | 必要 | 說明 |
|------|------|------|
| `TRELLO_API_KEY` | Y | Trello API key |
| `TRELLO_TOKEN` | Y | Trello user token |
| `TRELLO_BOARD_ID` | N | Board ID（用於 context） |
| `TRELLO_WEBHOOK_SECRET` | N | HMAC-SHA1 簽名驗證 |
| `GITHUB_TOKEN` | N | GitHub PAT |
| `GITHUB_OWNER` | N | GitHub org/username |
| `GITHUB_REPO` | N | GitHub repo name |
| `LLM_API_KEY` | N | Gemini/Anthropic API key（留空則用 stub） |
| `LLM_PROVIDER` | N | `gemini`（預設）或 `anthropic` |
| `DATABASE_URL` | N | SQLite 路徑（預設 `./orchestrator.db`） |
| `BOT_TRELLO_MEMBER_IDS` | N | 逗號分隔的 bot ID/username 清單 |
| `AUTO_REPLY_ENABLED` | N | 自動回覆 Trello comment |
| `AUTO_CREATE_GITHUB_ISSUE` | N | 自動建立 GitHub issue |
| `AUTO_CREATE_PLAN` | N | 自動寫入 plan 檔案 |
| `PORT` | N | Server port（預設 `8080`） |

### Safe Mode

`AUTO_REPLY_ENABLED`、`AUTO_CREATE_GITHUB_ISSUE`、`AUTO_CREATE_PLAN` 預設皆為 `false`（safe mode）。處於 safe mode 時，服務會記錄 decision、更新本地狀態，但不會實際呼叫外部 API。

## 常見開發模式

### 新增一個 Decision Action

1. 在 `domain/llm.go` 為 `LLMResponse.Action` 新增字串常數
2. 在 `adapter/gateway/llm_client.go` `decideStub` 中加入匹配邏輯
3. 在 `usecase/webhook_interactor.go` 的 Action 區段（~line 178）加入執行邏輯
4. 在 `usecase/scenario_test.go` 新增測試案例
5. 在 `adapter/controller/trello_integration_test.go` 新增 HTTP 層測試

### LLMResponse Action 清單

| Action | 觸發條件 | 行為 |
|--------|----------|------|
| `update_state_only` | 無匹配 / 一般 fallback | 僅更新本地 WorkflowState |
| `reply_comment` | `!reply` / keyword `reply` / `clarify` | 回覆 Trello comment（需 `AUTO_REPLY_ENABLED`） |
| `create_github_issue` | `!issue` / keyword `create issue` / `github issue` | 建立 GitHub Issue（需 `AUTO_CREATE_GITHUB_ISSUE`） |
| `update_github_issue` | LLM 分析決定 | 更新關聯的 GitHub Issue |
| `create_plan` | `!plan` / keyword `create plan` / `implementation plan` | 寫入 plan 檔案（需 `AUTO_CREATE_PLAN`） |
| `update_plan` | LLM 分析決定 | 更新 plan 檔案 |
| `ask_kos` | keyword `run claude` / `run codex` | 標記需人工介入（無 API key 時） |
| `ignore` | keyword `ignore` | 完全忽略 |

## 已知限制

- `.gitignore` 缺少 binary `trello-orchestrator`（Makefile 產出）及 `usecase/docs/`（plan 測試產出）
- `gopls` 需手動安裝（`go install golang.org/x/tools/gopls@latest`）
- 已有 Trello webhook 註冊、簽名驗證邏輯，但需實際憑證才能端到端驗證
- LLM 真實 API 呼叫（Gemini / Anthropic）尚未經完整整合測試
