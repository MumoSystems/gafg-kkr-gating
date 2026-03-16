package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/go-github/v75/github"
)

func main() {

	// GitHub API configuration
	repoOwner := os.Getenv("REPO_OWNER")
	repo := os.Getenv("REPO")
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GitHub token not set")
	}

	ctx := context.Background()
	client := github.NewClient(nil).WithAuthToken(token)

	// Fetch open PRs
	openPRs, _, err := client.PullRequests.List(ctx, repoOwner, repo, &github.PullRequestListOptions{
		State: "open",
	})
	if err != nil {
		log.Fatalf("Failed to fetch open PRs: %s", err)
	}

	if len(openPRs) > 0 {
		log.Printf("Found %v PRs", len(openPRs))
	}

	for _, pr := range openPRs {
		log.Printf("Processing PR #%v", *pr.Number)
		// Fetch workflows for the repository
		workflows, _, err := client.Actions.ListWorkflows(ctx, repoOwner, repo, &github.ListOptions{})
		if err != nil {
			log.Fatalf("Failed to fetch workflows for PR #%d: %s\n", *pr.Number, err)
		}

		log.Printf("Found %v workflows", workflows.TotalCount)

		for _, workflow := range workflows.Workflows {
			// log.Printf("Workflow %v", *workflow.Name)
			if *workflow.Name == "Check Work Item Approval" {
				log.Printf("Workflow %v", *workflow.Name)
				checkRuns, _, err := client.Actions.ListWorkflowRunsByID(ctx, repoOwner, repo, *workflow.ID, &github.ListWorkflowRunsOptions{
					HeadSHA: *pr.Head.SHA,
				})
				if err != nil {
					log.Printf("Failed to fetch workflow runs for PR #%d: %s\n", *pr.Number, err)
					continue
				}

				if len(checkRuns.WorkflowRuns) > 0 {
					fmt.Printf("Found %d runs for PR #%d:\n", len(checkRuns.WorkflowRuns), *pr.Number)
					for _, checkRun := range checkRuns.WorkflowRuns {

						status := *checkRun.Status
						conclusion := *checkRun.Conclusion
						fmt.Printf("Run ID: %v Status: %s Conclusion: %s\n", *checkRun.ID, status, conclusion)

						if conclusion == "failure" {
							fmt.Printf("Workflow run %v for PR #%d: FAILED. Rerunning...\n", *checkRun.ID, *pr.Number)
							// Rerun the failed workflow
							resp, err := client.Actions.RerunWorkflowByID(ctx, repoOwner, repo, *checkRun.ID)
							if err != nil {
								log.Printf("Failed to rerun workflow %v for PR #%d: %s\n", *checkRun.ID, *pr.Number, err)
								continue
							}
							fmt.Printf("Rerun triggered for workflow %v. Response: %+v\n", *checkRun.ID, resp)
						} else if conclusion == "success" {
							fmt.Printf("Workflow run %v for PR #%d: PASSED\n", *checkRun.ID, *pr.Number)
						} else {
							fmt.Printf("Workflow run %v for PR #%d: Status: %s (Conclusion: %s)\n", *checkRun.ID, *pr.Number, status, conclusion)
						}
					}
				} else {
					log.Printf("No workflow runs found for PR #%d\n", *pr.Number)
				}
			}
		}
	}

}
