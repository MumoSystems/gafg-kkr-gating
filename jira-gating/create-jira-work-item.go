package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/google/go-github/v31/github"
	"github.com/imroc/req/v3"
	"github.com/posener/goaction"
	"github.com/posener/goaction/actionutil"
)

type JiraIssue struct {
	Fields JiraIssueFields `json:"fields"`
	Update struct {
		IssueLinks []struct {
			Add struct {
				Values []struct {
					Type struct {
						Name string `json:"name"`
					} `json:"type"`
					OutwardIssues []struct {
						Key string `json:"key"`
					} `json:"outwardIssues"`
				} `json:"values"`
			} `json:"add"`
		} `json:"issuelinks"`
	} `json:"update"`
}

type JiraIssueFields struct {
	Description struct {
		Type    string `json:"type"`
		Version int    `json:"version"`
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	} `json:"description"`
	GithubPRID  string `json:"customfield_10572"`
	RequestType string `json:"customfield_10010"`
	Type        struct {
		ID string `json:"id"`
	} `json:"issuetype"`
	Project struct {
		Key string `json:"key"`
	} `json:"project"`
	Summary string `json:"summary"`
	// AffectedServices []struct {
	// 	ID string `json:"id"`
	// } `json:"customfield_10039"`
	// ServiceNowRequestNumber string `json:"customfield_10605"`
	// ServiceNowSysID         string `json:"customfield_10638"`
	ApproverGroups []struct {
		Name string `json:"name"`
	} `json:"customfield_10080"`
}

type BulkIssueUpdate struct {
	IssueUpdates []struct {
		Fields     JiraIssueFields `json:"fields"`
		Transition struct {
			ID string `json:"id"`
		} `json:"transition"`
	} `json:"issueUpdates"`
}

type BulkIssueResponse struct {
	Issues []struct {
		ID         string `json:"id"`
		Key        string `json:"key"`
		Self       string `json:"self"`
		Transition struct {
			Status          int `json:"status"`
			ErrorCollection struct {
				ErrorMessages []string               `json:"errorMessages"`
				Errors        map[string]interface{} `json:"errors"`
			} `json:"errorCollection"`
		} `json:"transition"`
	} `json:"issues"`
	Errors []interface{} `json:"errors"`
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
	ctx := context.Background()
	jiraURL := os.Getenv("JIRA_URL")
	jiraAPIToken := os.Getenv("JIRA_API_TOKEN")
	jiraUser := os.Getenv("JIRA_USER_EMAIL_ADDRESS")
	jiraIssueDescription := os.Getenv("JIRA_ISSUE_DESCRIPTION")
	jiraIssueSummary := os.Getenv("JIRA_ISSUE_SUMMARY")
	jiraIssueTypeID := os.Getenv("JIRA_ISSUE_TYPE")
	jiraProject := os.Getenv("JIRA_PROJECT")
	// affectedServiceID := os.Getenv("AFFECTED_SERVICE_ID")
	requestTypeID := os.Getenv("REQUEST_TYPE_ID")
	token := os.Getenv("GITHUB_TOKEN")

	// snowURL := os.Getenv("SNOW_URL")
	// snowUser := os.Getenv("SNOW_USER")
	// snowPassword := os.Getenv("SNOW_PASSWORD")

	prEvent, err := goaction.GetPullRequest()
	if err != nil {
		log.Fatalf("Failed getting PR event information: %s", err)
	}

	gh := actionutil.NewClientWithToken(ctx, token)
	re := regexp.MustCompile(`[A-Z]{2,}-\d+`)
	commits, _, err := gh.PullRequests.ListCommits(ctx, "mumosystems", "kkr-gating", *prEvent.Number, nil)
	if err != nil {
		log.Fatalf("Failed to fetch commits: %v", err)
	}

	issueKeys := make(map[string]bool)
	for _, commit := range commits {
		commitMessage := *commit.Commit.Message
		keys := re.FindAllString(commitMessage, -1)
		for _, key := range keys {
			issueKeys[key] = true
		}
	}

	var outwardIssues []struct {
		Key string `json:"key"`
	}
	for key := range issueKeys {
		outwardIssues = append(outwardIssues, struct {
			Key string `json:"key"`
		}{
			Key: key,
		})
	}

	githubPRID := strconv.Itoa(*prEvent.Number)
	approverGroups := []struct {
		Name string `json:"name"`
	}{
		{
			Name: "Change Approvers",
		},
	}

	// affectedServices := []struct {
	// 	ID string `json:"id"`
	// }{
	// 	{ID: affectedServiceID},
	// }

	// changeRequestNumber, changeRequestID, snowCreateErr := createServiceNowChangeRequest(snowURL, snowUser, snowPassword, jiraIssueSummary, jiraIssueDescription, githubPRID)
	// if snowCreateErr != nil {
	// 	log.Printf("Failed to create ServiceNow change request: %v", snowCreateErr)
	// 	return
	// }

	bulkIssuePayload := buildBulkIssuePayload(
		jiraIssueDescription,
		jiraIssueSummary,
		jiraIssueTypeID,
		requestTypeID,
		jiraProject,
		githubPRID,
		outwardIssues,
		approverGroups,
		// affectedServices,
		// changeRequestNumber,
		// changeRequestID,
	)

	client := getJIRAClient(jiraURL, jiraUser, jiraAPIToken)
	log.Printf("Payload: %v", bulkIssuePayload)
	var result BulkIssueResponse
	resp, err := client.R().
		SetContext(context.Background()).
		SetBody(&bulkIssuePayload).
		SetSuccessResult(&result).
		Post("/rest/api/3/issue/bulk")
	if err != nil {
		log.Printf("Failed to create issue: %v", err)
		return
	}
	if resp.IsErrorState() {
		log.Printf("Error response: %s", resp.String())
		return
	}
	if resp.StatusCode == 201 {
		log.Printf("Issues created: %+v", result)

		for _, issue := range result.Issues {
			// log.Printf("Issue created: ID: %s, Key: %s, Self: %s, Transition Status: %d", issue.ID, issue.Key, issue.Self, issue.Transition.Status)

			_, _, ghCommentErr := gh.IssuesCreateComment(
				ctx,
				*prEvent.Number,
				&github.IssueComment{
					Body: github.String(fmt.Sprintf("Jira change: %s/browse/%s", jiraURL, issue.Key)),
				},
			)

			if ghCommentErr != nil {
				log.Printf("Failed to create pr comment: %v", ghCommentErr)
				continue
			}

		}
	}
}

func buildBulkIssuePayload(
	jiraIssueDescription, jiraIssueSummary, jiraIssueTypeID, requestTypeID, jiraProject, githubPRID string,
	outwardIssues []struct {
		Key string `json:"key"`
	},
	approverGroups []struct {
		Name string `json:"name"`
	},
	// affectedServices []struct {
	// 	ID string `json:"id"`
	// },
	// serviceNowNumber string,
	// serviceNowID string,
) BulkIssueUpdate {
	issue := JiraIssue{
		Fields: JiraIssueFields{
			Description: struct {
				Type    string `json:"type"`
				Version int    `json:"version"`
				Content []struct {
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"content"`
			}{
				Type:    "doc",
				Version: 1,
				Content: []struct {
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				}{
					{
						Type: "paragraph",
						Content: []struct {
							Type string `json:"type"`
							Text string `json:"text"`
						}{
							{
								Type: "text",
								Text: jiraIssueDescription,
							},
						},
					},
				},
			},
			Type: struct {
				ID string `json:"id"`
			}{
				ID: jiraIssueTypeID,
			},
			Project: struct {
				Key string `json:"key"`
			}{
				Key: jiraProject,
			},
			Summary:     jiraIssueSummary,
			GithubPRID:  githubPRID,
			RequestType: requestTypeID,
			// AffectedServices:        affectedServices,
			// ServiceNowRequestNumber: serviceNowNumber,
			// ServiceNowSysID:         serviceNowID,
			ApproverGroups: approverGroups,
		},
	}

	if len(outwardIssues) > 0 {
		issue.Update.IssueLinks = []struct {
			Add struct {
				Values []struct {
					Type struct {
						Name string `json:"name"`
					} `json:"type"`
					OutwardIssues []struct {
						Key string `json:"key"`
					} `json:"outwardIssues"`
				} `json:"values"`
			} `json:"add"`
		}{
			{
				Add: struct {
					Values []struct {
						Type struct {
							Name string `json:"name"`
						} `json:"type"`
						OutwardIssues []struct {
							Key string `json:"key"`
						} `json:"outwardIssues"`
					} `json:"values"`
				}{
					Values: []struct {
						Type struct {
							Name string `json:"name"`
						} `json:"type"`
						OutwardIssues []struct {
							Key string `json:"key"`
						} `json:"outwardIssues"`
					}{
						{
							Type: struct {
								Name string `json:"name"`
							}{
								Name: "Blocks",
							},
							OutwardIssues: outwardIssues,
						},
					},
				},
			},
		}
	} else {
		issue.Update.IssueLinks = []struct {
			Add struct {
				Values []struct {
					Type struct {
						Name string `json:"name"`
					} `json:"type"`
					OutwardIssues []struct {
						Key string `json:"key"`
					} `json:"outwardIssues"`
				} `json:"values"`
			} `json:"add"`
		}{}
	}

	bulkIssueUpdate := BulkIssueUpdate{
		IssueUpdates: []struct {
			Fields     JiraIssueFields `json:"fields"`
			Transition struct {
				ID string `json:"id"`
			} `json:"transition"`
		}{
			{
				Fields: issue.Fields,
				Transition: struct {
					ID string `json:"id"`
				}{
					ID: "2",
				},
			},
		},
	}

	return bulkIssueUpdate
}
