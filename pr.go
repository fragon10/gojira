package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func createPullReq(issueID, issueTitle, branchName string) {
	cmd := exec.Command("gh", "pr", "list", "--base", branchName)
	output, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	if len(output) == 0 {
		templatePath := filepath.Join(".github", "gojira_template.md")
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
	}
}

func pr() {
	createPullReq("TEST-1", "Test issue", "test-branch")
}
