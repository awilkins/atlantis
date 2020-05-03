package vcs_test

import (
	"testing"

	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs"
	. "github.com/runatlantis/atlantis/testing"
)

func TestCodeCommitClient_MarkdownPullLink(t *testing.T) {
	client, err := vcs.NewCodeCommitClient()
	Ok(t, err)
	pull := models.PullRequest{
		BaseRepo: models.Repo{
			Name: "atlantis-test",
		},
		Num: 1,
	}
	s, err := client.MarkdownPullLink(pull)
	Ok(t, err)
	exp := "/codesuite/codecommit/repositories/atlantis-test/pull-requests/1"
	Equals(t, exp, s)
}
