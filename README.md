# Github LLM Triage Example with Swarmlet

This is a minimal example of using [Swarmlet](https://github.com/luisya22/swarmlet) to build an **AI-powered GitHub Issue Triage Agent**.

1. Search your GitHub repo for existing related issues.
2. If a match is found, respond with the URL of the existing issue.
3. If not, automatically create a new GitHub issue using your credentials.

<br>

## 🚀 Quick Start

### 1. Clone & Setup

```bash
git clone https://github.com/yourusername/swarmlet-github-triage-example.git
cd swarmlet-github-triage-example
go mod tidy
```

### 2. Environment Variables
Create a `.env` file in the project root:

```env
OPENAI_API_KEY=your_openai_api_key
GITHUB_TOKEN=your_github_token
GITHUB_OWNER=your_github_username_or_org
GITHUB_REPO=your_target_repo
```

- `OPEN_API_KEY`: Get one from [OpenAI Platform](https://platform.openai.com/)
- `GITHUB_TOKEN`: Needs `repo` scope to read/search/create issues
- The GitHub repo must exists and be accessible with your token.

### 3. Run the API Server

```bash
go run main.go
```

You should see:
```bash
Starting API server on port :8000
```
<br>
## ⚙️ How It Works

Depending on whether a similar issue already exists, it'll either find and return that or create a brand new one for you.

### Scenario 1: New Error (Issue Will Be Created)

```bash
curl -X POST http://localhost:8000/process_error \
  -H "Content-Type: application/json" \
  -d '{
    "error_log": "panic: unexpected nil pointer in database.go line 54"
}'
```
Response (new issue created):

```json
{
  "status": "success",
  "message": "GitHub issue created successfully! Title: \"Bug: Unexpected nil pointer in database\", URL: https://github.com/myorg/myrepo/issues/42",
  "issue_url": "https://github.com/myorg/myrepo/issues/42"
}
```
### Scenario 2: Duplicate Error (Existing Issue Found)
If you send the same log again, the agent will detect that this issue already exists:

```bash
curl -X POST http://localhost:8000/process_error \
  -H "Content-Type: application/json" \
  -d '{
    "error_log": "panic: unexpected nil pointer in database.go line 54"
}'
```

Response (existing issue found):

```json
{
  "status": "success",
  "message": "Found 1 existing issues:\n- Title: \"Bug: Unexpected nil pointer in database\", URL: https://github.com/myorg/myrepo/issues/42",
  "issue_url": "https://github.com/myorg/myrepo/issues/42"
}
```
<br>

## ⚠️ Warning
This project is intended as a demonstration and learning tool. It’s a minimal example meant to showcase how you can build LLM-powered workflows using Swarmlet.

Please don’t use this as-is in production. There’s no authentication, no rate limiting, and no protections against abuse or bad input.

 



