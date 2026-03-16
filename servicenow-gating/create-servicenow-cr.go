package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-github/v31/github"
	"github.com/imroc/req/v3"
	"github.com/posener/goaction"
	"github.com/posener/goaction/actionutil"
)

type ServiceNowChangeRequest struct {
	ShortDescription string   `json:"short_description"`
	Description      string   `json:"description"`
	AssignmentGroup  string   `json:"assignment_group"`
	State            string   `json:"state"`
	GithubPRID       string   `json:"u_github_pr_id"`
	NumberOfCommits  int      `json:"u_number_of_commits"`
	PRLink           string   `json:"u_pr_link"`
	PRTitle          string   `json:"u_pr_title"`
	PRAuthor         string   `json:"u_pr_author"`
	IssueKeys        []string `json:"u_issue_keys"`
}

func getServiceNowClient(snowURL, snowUser, snowPassword string) *req.Client {
	client := req.C().
		SetBaseURL(snowURL).
		SetCommonBasicAuth(snowUser, snowPassword).
		SetCommonHeader("Accept", "application/json").
		SetCommonHeader("Content-Type", "application/json")
	return client
}

func getCommitMessages(commits []*github.RepositoryCommit) string {
	var messages []string
	for _, commit := range commits {
		messages = append(messages, *commit.Commit.Message)
	}
	return strings.Join(messages, "\n\n")
}

func getIssueKeys(commits []*github.RepositoryCommit) []string {
	re := regexp.MustCompile(`[A-Z]{2,}-\d+`)
	issueKeys := make(map[string]bool)
	for _, commit := range commits {
		commitMessage := *commit.Commit.Message
		keys := re.FindAllString(commitMessage, -1)
		for _, key := range keys {
			issueKeys[key] = true
		}
	}

	var keys []string
	for key := range issueKeys {
		keys = append(keys, key)
	}
	return keys
}

func createServiceNowChangeRequest(snowURL, snowUser, snowPassword, githubPRID string, numCommits int, prLink, prTitle, prAuthor string, commits []*github.RepositoryCommit) (string, string, error) {
	commitMessages := getCommitMessages(commits)
	issueKeys := getIssueKeys(commits)
	payload := ServiceNowChangeRequest{
		ShortDescription: fmt.Sprintf("PR #%s: %s", githubPRID, prTitle),
		Description: fmt.Sprintf("PR Title: %s\nPR Author: %s\nPR Link: %s\nNumber of Commits: %d\n\nCommit Messages:\n%s\n\nIssue Keys: %s",
			prTitle, prAuthor, prLink, numCommits, commitMessages, strings.Join(issueKeys, ", ")),
		AssignmentGroup: "1cb8cd43c3d7321044257405e401311b",
		State:           "-4",
		GithubPRID:      githubPRID,
		NumberOfCommits: numCommits,
		PRLink:          prLink,
		PRTitle:         prTitle,
		PRAuthor:        prAuthor,
		IssueKeys:       issueKeys,
	}

	client := getServiceNowClient(snowURL, snowUser, snowPassword)

	var result map[string]interface{}
	resp, err := client.R().
		SetContext(context.Background()).
		SetBody(payload).
		SetSuccessResult(&result).
		Post("/api/now/table/change_request")
	if err != nil {
		log.Printf("Error creating change request: %v\n", err)
		os.Exit(1)
	}
	if resp.IsErrorState() {
		log.Printf("Error response from ServiceNow: %v\n", err)
		os.Exit(1)
	}

	resultMap := result["result"].(map[string]interface{})
	requestNumber := resultMap["number"].(string)
	sysID := resultMap["sys_id"].(string)

	return requestNumber, sysID, nil
}

func main() {
	ctx := context.Background()
	snowURL := os.Getenv("SNOW_URL")
	snowUser := os.Getenv("SNOW_USER")
	snowPassword := os.Getenv("SNOW_PASSWORD")
	repoOwner := os.Getenv("GITHUB_OWNER")
	githubRepo := os.Getenv("GITHUB_REPO")
	token := os.Getenv("GITHUB_TOKEN")

	prEvent, err := goaction.GetPullRequest()
	if err != nil {
		log.Fatalf("Failed getting PR event information: %s", err)
	}

	gh := actionutil.NewClientWithToken(ctx, token)
	commits, _, err := gh.PullRequests.ListCommits(ctx, repoOwner, githubRepo, *prEvent.Number, nil)
	if err != nil {
		log.Fatalf("Failed to fetch commits: %v", err)
	}

	numCommits := len(commits)
	prLink := fmt.Sprintf("https://github.com/%s/%s/pull/%d", repoOwner, githubRepo, *prEvent.Number)
	prTitle := prEvent.PullRequest.GetTitle()
	prAuthor := prEvent.PullRequest.GetUser().GetName()

	githubPRID := strconv.Itoa(*prEvent.Number)

	changeRequestNumber, _, snowCreateErr := createServiceNowChangeRequest(snowURL, snowUser, snowPassword, githubPRID, numCommits, prLink, prTitle, prAuthor, commits)
	if snowCreateErr != nil {
		log.Printf("Failed to create ServiceNow change request: %v", snowCreateErr)
		return
	}
	_, _, ghCommentErr := gh.IssuesCreateComment(
		ctx,
		*prEvent.Number,
		&github.IssueComment{
			Body: github.String(fmt.Sprintf("ServiceNow change request: %s \n\n ", fmt.Sprintf("%s/nav_to.do?uri=change_request.do?sysparm_query=number=%s", snowURL, changeRequestNumber))),
		},
	)

	if ghCommentErr != nil {
		log.Printf("Failed to create pr comment: %v", ghCommentErr)
		os.Exit(1)
	}
}
