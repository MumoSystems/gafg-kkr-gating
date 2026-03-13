package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"time"

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

type ServiceNowApprovalResponse struct {
	Results []struct {
		SysCreatedOn string `xml:"sys_created_on"`
		State        string `xml:"state"`
	} `xml:"result"`
}

func main() {

	jiraURL := os.Getenv("JIRA_URL")
	jiraAPIToken := os.Getenv("JIRA_API_TOKEN")
	jiraUser := os.Getenv("JIRA_USER_EMAIL_ADDRESS")

	snowURL := os.Getenv("SNOW_URL")
	snowUser := os.Getenv("SNOW_USER")
	snowPassword := os.Getenv("SNOW_PASSWORD")

	prEvent, err := goaction.GetPullRequest()
	if err != nil {
		log.Fatalf("Failed getting PR event information: %s", err)
	}

	jiraClient := req.C().
		SetTimeout(30 * time.Second)

	var searchResponse JiraSearchResponse

	jql := fmt.Sprintf(`"Github PR ID[Short Text]" ~ "%s"`, strconv.Itoa(*prEvent.Number))

	resp, err := jiraClient.R().
		SetSuccessResult(&searchResponse).
		SetHeader("Accept", "application/json").
		SetBasicAuth(jiraUser, jiraAPIToken).
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
			"- Key: %s\n  GitHub PR ID: %s\n  ServiceNow Sys ID: %s\n",
			issue.Key,
			issue.Fields.GithubPRID,
			issue.Fields.ServiceNowSysID,
		)

		approvalURL := fmt.Sprintf("%s/rest/servicedeskapi/request/%s/approval", jiraURL, issue.Key)
		var approvalResponse JSMApprovalResponse
		approvalResp, err := jiraClient.R().
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

		if issue.Fields.ServiceNowSysID != "" {
			snowClient := req.C().
				SetBaseURL(snowURL).
				SetCommonBasicAuth(snowUser, snowPassword).
				SetCommonHeader("Accept", "application/xml")

			serviceNowURL := fmt.Sprintf("%s/api/now/table/sysapproval_approver?sysparm_query=document_id=%s", snowURL, issue.Fields.ServiceNowSysID)
			var serviceNowResponse ServiceNowApprovalResponse
			serviceNowResp, err := snowClient.R().
				SetSuccessResult(&serviceNowResponse).
				Get(serviceNowURL)

			if err != nil {
				fmt.Printf("Error fetching ServiceNow approval details for issue %s: %v\n", issue.Key, err)
				os.Exit(1)
			}

			if !serviceNowResp.IsSuccessState() {
				fmt.Printf("Error fetching ServiceNow approval details for issue %s: %s\n", issue.Key, serviceNowResp.Status)
				os.Exit(1)
			}

			sort.Slice(serviceNowResponse.Results, func(i, j int) bool {
				return serviceNowResponse.Results[i].SysCreatedOn > serviceNowResponse.Results[j].SysCreatedOn
			})

			if len(serviceNowResponse.Results) == 0 {
				fmt.Printf("Error: No ServiceNow approvals found for document ID %s\n", issue.Fields.ServiceNowSysID)
				os.Exit(1)
			}

			fmt.Printf(
				"  Latest ServiceNow Approval State: %s\n  Created On: %s\n\n",
				serviceNowResponse.Results[0].State,
				serviceNowResponse.Results[0].SysCreatedOn,
			)

			if serviceNowResponse.Results[0].State != "approved" {
				fmt.Printf("Error: Latest ServiceNow approval for document ID %s is not 'approved'. Status: %s\n", issue.Fields.ServiceNowSysID, serviceNowResponse.Results[0].State)
				os.Exit(1)
			}
		} else {
			fmt.Printf("Error: ServiceNow Sys ID not found for issue %s\n", issue.Key)
			os.Exit(1)
		}
	}
}
