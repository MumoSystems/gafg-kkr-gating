package main

import (
	"fmt"
	"log"
	"os"

	"github.com/imroc/req/v3"
	"github.com/posener/goaction"
)

type ServiceNowResponse struct {
	Result struct {
		Number     string `xml:"number"`
		State      string `xml:"state"`
		GithubPRID string `xml:"u_github_pr_id"`
		Approval   string `xml:"approval"`
	} `xml:"result"`
}

func getSNOWClient(snowURL, snowUser, snowPassword string) *req.Client {
	client := req.C().
		SetBaseURL(snowURL).
		SetCommonBasicAuth(snowUser, snowPassword).
		SetCommonHeader("Accept", "application/xml").
		SetCommonHeader("Content-Type", "application/xml")
	return client
}

func main() {
	snowURL := os.Getenv("SNOW_URL")
	snowUser := os.Getenv("SNOW_USER")
	snowPassword := os.Getenv("SNOW_PASSWORD")

	prEvent, err := goaction.GetPullRequest()
	if err != nil {
		log.Fatalf("Failed getting PR event information: %s", err)
	}

	client := getSNOWClient(snowURL, snowUser, snowPassword)

	queryURL := fmt.Sprintf("%s/api/now/table/change_request?sysparm_query=u_github_pr_id=%v", snowURL, *prEvent.Number)
	log.Printf("Query: %s", queryURL)

	var response ServiceNowResponse
	resp, err := client.R().
		SetSuccessResult(&response).
		Get(queryURL)

	if err != nil {
		fmt.Printf("Error fetching change requests: %v\n", err)
		os.Exit(1)
	}

	if !resp.IsSuccessState() {
		fmt.Printf("Error: %s\n", resp.Status)
		os.Exit(1)
	}

	log.Printf("Response: %v", response)

	fmt.Printf(
		"Change Request Number: %s\nState: %s\nApproval: %s\n",
		response.Result.Number,
		response.Result.State,
		response.Result.Approval,
	)

	if response.Result.Approval != "approved" {
		fmt.Printf("Error: Change request %s is not 'approved'. Status: %s\n", response.Result.Number, response.Result.Approval)
		os.Exit(1)
	}
}
