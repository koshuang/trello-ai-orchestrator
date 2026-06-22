# Trello AI Webhook & Orchestrator Service

A durable workflow webhook orchestrator for Trello AI operations, built in Go using Clean Architecture.

---

## 1. Clean Architecture Design

This service is structured to decouple core business logic from database systems, HTTP routers, and external APIs:

* **`domain/`**: Pure data entities (`WorkflowState`, `ProcessedEvent`, `TrelloCardContext`, `LLMResponse`). Contains zero dependencies.
* **`usecase/`**: Business logic. Declares interfaces (`interfaces.go`) and manages the orchestrator flow (`webhook_interactor.go`).
* **`adapter/`**: Implements transport and client interfaces:
  * `controller/`: Gin HTTP routes (`webhook_handler.go`, `health_handler.go`).
  * `repository/`: SQLite CRUD commands (`sqlite_repo.go`).
  * `gateway/`: Clients for external integrations (`trello_client.go`, `github_client.go`, `llm_client.go`).
* **`infrastructure/`**: Configures database storage and environment parsing.

---

## 2. Environment Variables

Create a `.env` file based on `.env.example`:

```env
# Trello Credentials
TRELLO_API_KEY=your_key
TRELLO_TOKEN=your_token
TRELLO_BOARD_ID=your_board_id
TRELLO_WEBHOOK_SECRET=your_signature_secret # Optional, verifies webhook origin

# GitHub API Credentials
GITHUB_TOKEN=ghp_your_token
GITHUB_OWNER=your_github_org_or_username
GITHUB_REPO=your_github_repo_name

# LLM Configuration
LLM_PROVIDER=gemini # Supports "gemini" (default) or "anthropic"
LLM_API_KEY=your_llm_api_key # Optional, falls back to a rules-based stub engine if empty

# SQLite Connection
DATABASE_URL=./orchestrator.db

# bot users to ignore (comma-separated list of member IDs/usernames)
BOT_TRELLO_MEMBER_IDS=bot_username_1,64f123abc4567

# Safe Mode flags (Default: false)
AUTO_REPLY_ENABLED=false
AUTO_CREATE_GITHUB_ISSUE=false
AUTO_CREATE_PLAN=false

PORT=8080
```

---

## 3. How to Run Locally

### Using Go Toolchain
1. Build the binary:
   ```bash
   go build -o trello-orchestrator main.go
   ```
2. Run the application:
   ```bash
   ./trello-orchestrator
   ```

---

## 4. How to Register Trello Webhook

Trello requires an active HTTP callback URL to register webhooks. During registration, Trello will fire a `HEAD` request to verify the route is valid and responsive.

1. Expose your localhost port `8080` (e.g. using `ngrok`):
   ```bash
   ngrok http 8080
   ```
   Assume ngrok gives you `https://1234.ngrok-free.app`.
2. Register the webhook by sending a request to Trello's REST API:
   ```bash
   curl -X POST -H "Content-Type: application/json" \
     "https://api.trello.com/1/webhooks/?key=YOUR_TRELLO_KEY&token=YOUR_TRELLO_TOKEN" \
     -d '{
       "description": "Trello AI Orchestrator Webhook",
       "callbackURL": "https://1234.ngrok-free.app/webhooks/trello",
       "idModel": "YOUR_TRELLO_BOARD_ID"
     }'
   ```
3. Once registered, Trello will automatically trigger the `/webhooks/trello` route on every card update/comment.

---

## 5. How to Test with Sample Payload

To test the application locally without Trello triggering it:

1. Start your local server (with `LLM_API_KEY` empty to trigger the mock decision engine).
2. Send a sample comment payload using curl:
   ```bash
   curl -X POST http://localhost:8080/webhooks/trello \
     -H "Content-Type: application/json" \
     -d '{
       "action": {
         "id": "mock_action_001",
         "type": "commentCard",
         "date": "2026-06-22T08:00:00Z",
         "memberCreator": {
           "id": "user_456",
           "username": "developer_john",
           "fullName": "John Doe"
         },
         "data": {
           "text": "please create issue for database schema validation",
           "card": {
             "id": "card_abc123",
             "name": "Database Schema Setup",
             "shortLink": "xYz123"
           }
         }
       }
     }'
   ```
3. Check the response. The CLI server logs will output:
   * Action determined (e.g. `create_github_issue`)
   * Current safe mode status
   * Stored SQLite state updates
4. Query the API directly to inspect the saved state:
   ```bash
   curl http://localhost:8080/workflow-states/card_abc123
   ```

---

## 6. Remaining TODOs

1. **Verify Trello callback URL schema**: Ensure `verifyTrelloSignature` perfectly matches the schema format that Trello forwards (HTTP vs HTTPS) behind load balancers/reverse proxies.
2. **Add support for `updateCard` and `addAttachmentToCard`**: Expand controllers and interactor to parse attachments and description edits to capture manual links to GitHub/plans.
3. **Write plans directly to GitHub/PRs**: If local filesystem write access is restricted, adapt `writePlanFile` to commit directly to GitHub via its Repository Contents API.
4. **Deploy using Docker**: Pack the binary inside a minimal scratch/alpine Docker image for container environments.
