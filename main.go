package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var EMPTY_COMMIT_MESSAGE = "[skip ci] REMOVE ME. EMPTY COMMIT"

type Component struct {
	Name string `json:"name"`
}

type JiraIssue struct {
	IssueID string `json:"key"`
	Fields  struct {
		Summary    string      `json:"summary"`
		Components []Component `json:"components"`
	} `json:"fields"`
}

type JiraResponse struct {
	Issues []JiraIssue `json:"issues"`
}

func getJiraIssues(jiraAPIToken, jiraDomain, bID, bStatus string) []JiraIssue {
	jql := fmt.Sprintf("project=%s AND status=%s", bID, bStatus)
	url := fmt.Sprintf("https://%s/rest/api/2/search?jql=%s", jiraDomain, url.QueryEscape(jql))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+jiraAPIToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	var jiraResponse JiraResponse
	err = json.Unmarshal(body, &jiraResponse)
	if err != nil {
		log.Fatal(err)
	}

	return jiraResponse.Issues
}

func selectJiraIssue(issues []JiraIssue, iFilter string) (string, string) {
	keys := make(map[string]JiraIssue)
	var fzfInput []string
	for _, issue := range issues {
		for _, component := range issue.Fields.Components {
			if component.Name == iFilter {
				key := fmt.Sprintf("%s - %s", issue.IssueID, issue.Fields.Summary)
				keys[key] = issue
				fzfInput = append(fzfInput, key)
				break
			}
		}
	}

	cmd := exec.Command("fzf", "--height", "50%", "--prompt", "Select a Jira issue: ")
	cmd.Stdin = strings.NewReader(strings.Join(fzfInput, "\n"))
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		log.Fatalf("Error running fzf: %v", err)
	}

	selectedKey := strings.TrimSpace(string(output))
	selectedIssue := keys[selectedKey]

	return selectedIssue.IssueID, selectedIssue.Fields.Summary
}

func createBranch(issueID, issueTitle string) {
	log.Println("Creating branch for issue: " + issueID)

	sanitizedIssueTitle := strings.ReplaceAll(issueTitle, " ", "-")
	reg, err := regexp.Compile("[^a-zA-Z0-9-]+")
	if err != nil {
		log.Fatal(err)
	}
	sanitizedIssueTitle = reg.ReplaceAllString(sanitizedIssueTitle, "")
	sanitizedIssueTitle = strings.ToLower(sanitizedIssueTitle)

	branchName := fmt.Sprintf("%s/%s", issueID, sanitizedIssueTitle)
	parentcmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	parentSHA1, err := parentcmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	if err := exec.Command("git", "rev-parse", "--verify", branchName).Run(); err != nil {
		err = exec.Command("git", "switch", "-c", branchName).Run()
		if err != nil {
			log.Fatal(err)

		}
		log.Println("Branch created: " + branchName)

	} else {
		if err := exec.Command("git", "switch", branchName).Run(); err != nil {
			log.Fatal(err)
		}

		log.Println("Branch switched: " + branchName)
	}

	createCommit(issueID, issueTitle, branchName, parentSHA1)
}

func createCommit(issueID string, issueTitle string, branchName string, parent []byte) {
	log.Println("Creating commit for issue: " + issueID)

	cmd := exec.Command("git", "rev-parse", "--verify", "HEAD")
	current, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	if bytes.Equal(current, parent) {
		cmd = exec.Command("git", "commit", "--allow-empty", "-m", EMPTY_COMMIT_MESSAGE, "--no-verify")
		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}

		log.Println("Empty commit created: " + EMPTY_COMMIT_MESSAGE)

		setRemoteUpstream(issueID, issueTitle, branchName)
	}
}

func setRemoteUpstream(issueID, issueTitle, branchName string) {
	log.Println("Setting remote upstream for branch: " + branchName)
	cmd := exec.Command("git", "push", "-u", "origin", branchName)
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Remote upstream set: " + "origin/" + branchName)

	createPR(issueID, issueTitle, branchName)
}

func createPR(issueID, issueTitle, branchName string) {
	log.Println("Checking for pull request for issue: " + issueID)

	cmd := exec.Command("gh", "pr", "list", "--base", branchName)
	output, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}
	if strings.TrimSpace(string(output)) == "" {
		log.Println("Creating pull request for issue: " + issueID)

		templatePath := filepath.Join(".github", "gojira_pr_template.md")
		title := fmt.Sprintf("%s: %s", issueID, issueTitle)
		createPrCmd := exec.Command("gh", "pr", "create", "-d", "-t", title)
		body := ""
		if _, err := os.Stat(templatePath); err == nil {
			template, err := os.ReadFile(templatePath)
			if err != nil {
				log.Fatal(err)
			}
			body = strings.Replace(string(template), "{{ISSUE_ID}}", issueID, -1)
			createPrCmd.Args = append(createPrCmd.Args, "-b", body)
		} else {
			createPrCmd.Args = append(createPrCmd.Args, "-b", "")
		}

		createPrCmd.Stdout = os.Stdout
		createPrCmd.Stderr = os.Stderr
		err = createPrCmd.Run()
		if err != nil {
			log.Fatal(err)
		}

		log.Println("Pull request created: " + title)
	}
}

func main() {
	jiraAPIToken := os.Getenv("JIRA_API_TOKEN")
	if jiraAPIToken == "" {
		log.Fatal("Please provide a Jira API Token")
	}

	jiraDomain := os.Getenv("JIRA_DOMAIN")
	if jiraDomain == "" {
		log.Fatal("Please provide a Jira Domain")
	}

	var boardID = "UTPR"                     // Utsikt - Presence Current Sprint.
	var boardStatus = "'To Do'"              // Issue Status, must be in single quotes.
	var issueFilter = "Software Engineering" // Issue Filter.

	jiraIssues := getJiraIssues(jiraAPIToken, jiraDomain, boardID, boardStatus)
	if len(jiraIssues) == 0 {
		log.Fatal("No Jira issues with 'To Do' status found.")
	}

	issueID, issueSummary := selectJiraIssue(jiraIssues, issueFilter)
	if issueID == "" || issueSummary == "" {
		log.Fatal("No Jira issue selected. Manual entry is required.")
	}

	// issueID := "TEST-5"
	// issueTitle := "Test5"
	// branchName := "TEST-1/test"

	// createBranch(issueID, issueTitle)
	createBranch(issueID, issueSummary)
}
