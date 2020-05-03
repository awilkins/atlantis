package vcs

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/codecommit"
	"github.com/pkg/errors"

	"github.com/runatlantis/atlantis/server/events/models"
)

type CodeCommitClient struct {
	client *codecommit.CodeCommit
}

func (c *CodeCommitClient) GetModifiedFiles(repo models.Repo, pull models.PullRequest) ([]string, error) {
	return []string{}, errors.New("Not Implemented")
}

func (c *CodeCommitClient) CreateComment(repo models.Repo, pullNum int, comment string) error {
	return errors.New("Not Implemented")
	// API : PostCommentForPullRequest
}

func (c *CodeCommitClient) HidePrevPlanComments(repo models.Repo, pullNum int) error {
	return errors.New("Not Implemented")
	// API : Not sure CC has an equivalent to minimizeComment()
	// but you can do DeleteCommentContent and UpdateComment
}

func (c *CodeCommitClient) PullIsApproved(repo models.Repo, pull models.PullRequest) (bool, error) {
	return false, errors.New("Not Implemented")
	// API : EvaluatePullRequestApprovalRules
}

func (c *CodeCommitClient) PullIsMergeable(repo models.Repo, pull models.PullRequest) (bool, error) {
	return false, errors.New("Not Implemented")
	// API : GetMergeConflicts
}

func (c *CodeCommitClient) UpdateStatus(repo models.Repo, pull models.PullRequest, state models.CommitStatus, src string, description string, url string) error {
	return errors.New("Not Implemented")
	// API possibilities
	// https://github.blog/2012-09-04-commit-status-api/
	// UpdatePullRequestApprovalState
}

func (c *CodeCommitClient) MergePull(pull models.PullRequest) error {
	return errors.New("Not Implemented")
	// API : MergePullRequestBy[ThreeWay|FastForward|Squash]
}

func (c *CodeCommitClient) MarkdownPullLink(pull models.PullRequest) (string, error) {
	// Probably just going to have to be the full link
	// e.g. https://eu-west-2.console.aws.amazon.com/codesuite/codecommit/repositories/atlantis-test/pull-requests/2/details?region=eu-west-2
	// For repo in the same acct/region the path works
	// e.g. /codesuite/codecommit/repositories/atlantis-test/pull-requests/2
	return fmt.Sprintf("/codesuite/codecommit/repositories/%s/pull-requests/%d", pull.BaseRepo.Name, pull.Num), nil
}

func NewCodeCommitClient() (*CodeCommitClient, error) {
	return nil, nil
}
