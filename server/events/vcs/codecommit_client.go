package vcs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/service/codecommit"
	"github.com/aws/aws-sdk-go/service/codecommit/codecommitiface"
	"github.com/pkg/errors"

	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs/common"
)

type CodeCommitClient struct {
	Client  codecommitiface.CodeCommitAPI
	UserArn string
}

func (c *CodeCommitClient) GetModifiedFiles(repo models.Repo, pull models.PullRequest) ([]string, error) {
	return []string{}, errors.New("Not Implemented")
}

const commentSizeLimit int = 1000
const sepEnd string = "\n```\n</details>" +
	"\n<br>\n\n**Warning**: Output length greater than max comment size. Continued in next comment."
const sepStart string = "Continued from previous comment.\n<details><summary>Show Output</summary>\n\n" +
	"```diff\n"

func (c *CodeCommitClient) CreateComment(repo models.Repo, pullNum int, comment string) error {
	// API : PostCommentForPullRequest

	repoName := repo.Name
	pullRequestId := strconv.Itoa(pullNum)

	pullRequest, err := c.Client.GetPullRequest(&codecommit.GetPullRequestInput{
		PullRequestId: &pullRequestId,
	})
	if err != nil {
		return err
	}

	baseCommitId := *pullRequest.PullRequest.PullRequestTargets[0].MergeBase
	prBranchTipId := *pullRequest.PullRequest.PullRequestTargets[0].SourceCommit

	comments := common.SplitComment(comment, commentSizeLimit, sepEnd, sepStart)
	for _, commentFragment := range comments {
		_, err := c.Client.PostCommentForPullRequest(&codecommit.PostCommentForPullRequestInput{
			RepositoryName: &repoName,
			PullRequestId:  &pullRequestId,
			BeforeCommitId: &baseCommitId,
			AfterCommitId:  &prBranchTipId,
			Content:        &commentFragment,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *CodeCommitClient) HidePrevPlanComments(repo models.Repo, pullNum int) error {
	var allComments []*codecommit.Comment
	pullRequestId := strconv.Itoa(pullNum)
	var nextToken *string = nil
	for {
		response, err := c.Client.GetCommentsForPullRequest(&codecommit.GetCommentsForPullRequestInput{
			PullRequestId: &pullRequestId,
			NextToken:     nextToken,
		})
		if err != nil {
			return err
		}
		allComments = append(allComments, response.CommentsForPullRequestData[0].Comments...)
		if response.NextToken == nil {
			break
		}
		nextToken = response.NextToken
	}

	for _, comment := range allComments {
		if comment.AuthorArn != nil && !strings.EqualFold(*comment.AuthorArn, c.UserArn) {
			continue
		}
		body := strings.Split(*comment.Content, "\n")
		if len(body) == 0 {
			continue
		}
		firstLine := strings.ToLower(body[0])
		if !strings.Contains(firstLine, models.PlanCommand.String()) {
			continue
		}
		_, err := c.Client.DeleteCommentContent(&codecommit.DeleteCommentContentInput{
			CommentId: comment.CommentId,
		})
		if err != nil {
			return errors.Wrapf(err, "minimize conmment %s", *comment.CommentId)
		}
	}

	return nil
}

func (c *CodeCommitClient) PullIsApproved(repo models.Repo, pull models.PullRequest) (bool, error) {

	pullRequestId := strconv.Itoa(pull.Num)

	pr, err := c.Client.GetPullRequest(&codecommit.GetPullRequestInput{
		PullRequestId: &pullRequestId,
	})
	if err != nil {
		return false, err
	}
	revisionId := *pr.PullRequest.RevisionId
	eval, err := c.Client.EvaluatePullRequestApprovalRules(&codecommit.EvaluatePullRequestApprovalRulesInput{
		PullRequestId: &pullRequestId,
		RevisionId:    &revisionId,
	})
	if err != nil {
		return false, err
	}
	return *eval.Evaluation.Approved, nil
}

func (c *CodeCommitClient) PullIsMergeable(repo models.Repo, pull models.PullRequest) (bool, error) {
	// API : GetMergeConflicts

	repoName := pull.BaseRepo.Name
	destinationSpec := pull.BaseBranch
	sourceSpec := pull.HeadBranch
	mergeOption := codecommit.MergeOptionTypeEnumThreeWayMerge
	conflictDetail := codecommit.ConflictDetailLevelTypeEnumLineLevel

	response, err := c.Client.GetMergeConflicts(&codecommit.GetMergeConflictsInput{
		RepositoryName:             &repoName,
		DestinationCommitSpecifier: &destinationSpec,
		SourceCommitSpecifier:      &sourceSpec,
		MergeOption:                &mergeOption,
		ConflictDetailLevel:        &conflictDetail,
	})
	if err != nil {
		return false, err
	}
	return *response.Mergeable, nil
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

type mockCodeCommit struct {
	codecommitiface.CodeCommitAPI
}

func NewCodeCommitClient() (*CodeCommitClient, error) {
	return nil, nil
}
