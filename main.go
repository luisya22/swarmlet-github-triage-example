package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"github.com/luisya22/swarmlet"
	"golang.org/x/oauth2"
)

type ErrorLogRequest struct {
	ErrorLog string `json:"error_log"`
}

type APIResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	IssueURL string `json:"issue_url,omitempty"`
}

var (
	ghClient    *github.Client
	ghOwner     string
	ghRepo      string
	llmPipeline *swarmlet.Pipeline
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: No .env file found or error loading: %v", err)
	}

	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	githubToken := os.Getenv("GITHUB_TOKEN")
	ghOwner = os.Getenv("GITHUB_OWNER")
	ghRepo = os.Getenv("GITHUB_REPO")

	if openaiAPIKey == "" || githubToken == "" || ghOwner == "" || ghRepo == "" {
		log.Fatal("Error: OPENAI_API_KEY, GITHUB_TOKEN, GITHUB_OWNER, and GITHUB_REPO environment variables must be set.")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	ghClient = github.NewClient(tc)

	initializeAIPipeline(openaiAPIKey)

	http.HandleFunc("POST /process_error", handleProcessError)
	port := ":8000"
	log.Printf("Starting API server on port %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

func initializeAIPipeline(openaiAPIKey string) {
	tools := []swarmlet.LLMTool{
		{
			Name:        "search_github_issues",
			Description: "Searches for existing GitHub issues in the repository based on a query. Returns a list of issue titles and URLs if found, otherwise indicates no issues found.",
			Params: map[string]swarmlet.LLMToolFieldProperty{
				"query": {
					Type:        "string",
					Description: "The search query for GitHub issues, e.g., 'bug in login module' or 'database connection error'.",
				},
			},
			Executor: searchGithubIssues,
		},
		{
			Name:        "create_github_issue",
			Description: "Creates a new GitHub issue in the specified repository. Provide a title, detailed body, and labels.",
			Params: map[string]swarmlet.LLMToolFieldProperty{
				"title": {
					Type:        "string",
					Description: "The title of the new GitHub issue (e.g., 'Bug: Login failure on homepage').",
				},
				"body": {
					Type:        "string",
					Description: "The detailed description for the GitHub issue, including stack traces or context.",
				},
				"labels": {
					Type:        "array",
					Description: "An array of labels to apply to the issue, e.g., ['bug', 'llm created'].",
					Enum:        []string{"bug", "llm created", "enhancement"},
				},
			},
			Executor: createGithubIssues,
		},
	}

	systemPrompt := fmt.Sprintf(agentSystemPrompt, ghOwner, ghRepo)

	llm := swarmlet.NewOpenAILLM(openaiAPIKey, "gpt-4o-mini")
	memory := swarmlet.NewDummyMemory()

	augmentedNode := swarmlet.NewAugmentedLLMNode(
		swarmlet.WithAugmentedID("github-triage-agent"),
		swarmlet.WithAugmentedSystemPrompt(systemPrompt),
		swarmlet.WithAugmentedTools(tools...),
	)

	llmPipeline = swarmlet.NewPipeline("GitHubIssueTriage", augmentedNode, llm, memory)
}

func handleProcessError(w http.ResponseWriter, r *http.Request) {
	var req ErrorLogRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.ErrorLog == "" {
		http.Error(w, "Error log cannot be empty", http.StatusBadRequest)
		return
	}

	var outputBuffer bytes.Buffer
	finalOutput, err := llmPipeline.Run(r.Context(), req.ErrorLog, "run-id"+req.ErrorLog[:10], &outputBuffer)
	if err != nil {
		log.Printf("Pipeline execution failed: %v", err)
		http.Error(w, fmt.Sprintf("Agent failed to process error: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Agent's final response: %s", finalOutput)

	resp := APIResponse{
		Status:  "success",
		Message: finalOutput,
	}

	// Try to parse the issue URL from the final output for convenience
	if strings.Contains(finalOutput, "GitHub issue created successfully!") {
		if idx := strings.Index(finalOutput, "URL: "); idx != -1 {
			if endIdx := strings.IndexAny(finalOutput[idx+5:], " \n"); endIdx != -1 {
				resp.IssueURL = strings.TrimSpace(finalOutput[idx+5 : idx+5+endIdx])
			} else {
				resp.IssueURL = strings.TrimSpace(finalOutput[idx+5:])
			}
		}
	} else if strings.Contains(finalOutput, "Found existing issues:") {
		// If it's an existing issue, try to extract the first URL if present
		if idx := strings.Index(finalOutput, "URL: "); idx != -1 {
			if endIdx := strings.IndexAny(finalOutput[idx+5:], " \n"); endIdx != -1 {
				resp.IssueURL = strings.TrimSpace(finalOutput[idx+5 : idx+5+endIdx])
			} else {
				resp.IssueURL = strings.TrimSpace(finalOutput[idx+5:])
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func searchGithubIssues(args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'query' argument for search_github_issues")
	}
	log.Printf("Tool Call: Searching for GitHub issues for query: '%s'", query)

	searchQuery := fmt.Sprintf("%s is:issue in:title,body repo:%s/%s", query, ghOwner, ghRepo)
	issues, _, err := ghClient.Search.Issues(context.Background(), searchQuery, nil)
	if err != nil {
		log.Printf("Error searching GitHub issues: %v", err)
		return fmt.Sprintf("Error searching GitHub issues: %v", err), err
	}

	if len(issues.Issues) == 0 {
		return "No existing issues found for this query.", nil
	}

	var results []string
	for _, issue := range issues.Issues {
		results = append(results, fmt.Sprintf("- Title: \"%s\", URL: %s", *issue.Title, *issue.HTMLURL))
	}
	return fmt.Sprintf("Found %d existing issues:\n%s", len(issues.Issues), strings.Join(results, "\n")), nil

}

func createGithubIssues(args map[string]any) (string, error) {
	title, ok := args["title"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'title' argument for create_github_issue")
	}

	body, ok := args["body"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'body' argument for create_github_issue")
	}

	labelsRaw, ok := args["labels"].([]any)
	if !ok {
		labelsRaw = []any{}
	}

	var labels []string
	for _, l := range labelsRaw {
		if s, isString := l.(string); isString {
			labels = append(labels, s)
		}
	}

	log.Printf("Tool Call: Creating GitHub issue - Title: '%s', Labels: %v", title, labels)

	newIssue := &github.IssueRequest{
		Title:  &title,
		Body:   &body,
		Labels: &labels,
	}

	issue, _, err := ghClient.Issues.Create(context.Background(), ghOwner, ghRepo, newIssue)
	if err != nil {
		log.Printf("Error creating GitHub issue: %v", err)
		return fmt.Sprintf("Error creating GitHub issue: %v", err), err
	}

	return fmt.Sprintf("GitHub issue created successfully! Title: \"%s\", URL: %s", *issue.Title, *issue.HTMLURL), nil
}
