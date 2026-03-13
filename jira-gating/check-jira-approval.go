package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"

	"github.com/imroc/req/v3"
	"github.com/posener/goaction"
)

type JiraSearchResponse struct {
	IsLast bool `json:"isLast"`
	Issues []struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
			Status  struct {
				Name string `json:"name"`
			} `json:"status"`
			Description struct {
				Content []struct {
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"content"`
			} `json:"description"`
			GithubPRID      string `json:"customfield_10572"`
			ServiceNowSysID string `json:"customfield_10638"`
		} `json:"fields"`
	} `json:"issues"`
}

type JSMApprovalResponse struct {
	Values []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		FinalDecision string `json:"finalDecision"`
	} `json:"values"`
}

func getJIRAClient(jiraURL, jiraUser, jiraAPIToken string) *req.Client {
	client := req.C().
		SetBaseURL(jiraURL).
		SetCommonBasicAuth(jiraUser, jiraAPIToken).
		SetCommonHeader("Accept", "application/json").
		SetCommonHeader("Content-Type", "application/json")
	return client
}

func main() {
	jiraURL := os.Getenv("JIRA_URL")
	jiraAPIToken := os.Getenv("JIRA_API_TOKEN")
	jiraUser := os.Getenv("JIRA_USER_EMAIL_ADDRESS")

	prEvent, err := goaction.GetPullRequest()
	if err != nil {
		log.Fatalf("Failed getting PR event information: %s", err)
	}

	var searchResponse JiraSearchResponse

	jql := fmt.Sprintf(`"Github PR ID[Short Text]" ~ "%s"`, strconv.Itoa(*prEvent.Number))

	client := getJIRAClient(jiraURL, jiraUser, jiraAPIToken)
	resp, err := client.R().
		SetContext(context.Background()).
		SetSuccessResult(&searchResponse).
		SetQueryParams(
			map[string]string{
				"jql":    jql,
				"fields": "customfield_10572,customfield_10638",
			},
		).
		Get(jiraURL + "/rest/api/3/search/jql")

	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		os.Exit(1)
	}

	if !resp.IsSuccessState() {
		fmt.Printf("Error: %s\n", resp.Status)
		os.Exit(1)
	}

	if len(searchResponse.Issues) == 0 {
		fmt.Printf("No issues found for GitHub PR ID: %s\n", strconv.Itoa(*prEvent.Number))
		os.Exit(1)
	}

	fmt.Printf("Found %d issue(s) for GitHub PR ID %s:\n", len(searchResponse.Issues), strconv.Itoa(*prEvent.Number))

	for _, issue := range searchResponse.Issues {
		fmt.Printf(
			"- Key: %s\n  GitHub PR ID: %s\n",
			issue.Key,
			issue.Fields.GithubPRID,
		)

		approvalURL := fmt.Sprintf("%s/rest/servicedeskapi/request/%s/approval", jiraURL, issue.Key)
		var approvalResponse JSMApprovalResponse
		approvalResp, err := client.R().
			SetSuccessResult(&approvalResponse).
			SetHeader("Accept", "application/json").
			SetBasicAuth(jiraUser, jiraAPIToken).
			Get(approvalURL)

		if err != nil {
			fmt.Printf("Error fetching approval details for issue %s: %v\n", issue.Key, err)
			os.Exit(1)
		}

		if !approvalResp.IsSuccessState() {
			fmt.Printf("Error fetching approval details for issue %s: %s\n", issue.Key, approvalResp.Status)
			os.Exit(1)
		}

		sort.Slice(approvalResponse.Values, func(i, j int) bool {
			return approvalResponse.Values[i].ID > approvalResponse.Values[j].ID
		})

		if len(approvalResponse.Values) == 0 {
			fmt.Printf("Error: No approvals found for work item %s\n", issue.Key)
			os.Exit(1)
		}
		log.Println(approvalResponse.Values)

		fmt.Printf(
			"  Latest Approval ID: %s\n  Name: %s\n  Final Decision: %s\n\n",
			approvalResponse.Values[0].ID,
			approvalResponse.Values[0].Name,
			approvalResponse.Values[0].FinalDecision,
		)

		if approvalResponse.Values[0].FinalDecision != "approved" {
			fmt.Printf("Error: Latest approval for work item %s is not 'approved'. Status: %s\n", issue.Key, approvalResponse.Values[0].FinalDecision)
			os.Exit(1)
		}

	}
}
