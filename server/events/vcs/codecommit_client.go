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

const (
	CodeCommitUserFlag = "codecommit-user"
)

// var stringFlags = map[string]cmd.StringFlag{
// 	CodeCommitUserFlag: {
// 		description: "CodeCommit user name. Since the user you're operating as is" +
// 			" determined by your IAM credentials/role, this may just be a signal to turn on CodeCommit right now",
// 	},
// }

// var intFlags = map[string]cmd.IntFlag{}

// var boolFlags = map[string]cmd.BoolFlag{}

type CodeCommitClient struct {
	Client  codecommitiface.CodeCommitAPI
	UserArn string
}

func (c *CodeCommitClient) GetModifiedFiles(repo models.Repo, pull models.PullRequest) ([]string, error) {
	mergeOption := codecommit.MergeOptionTypeEnumThreeWayMerge
	mergeOutput, err := c.Client.CreateUnreferencedMergeCommit(&codecommit.CreateUnreferencedMergeCommitInput{
		RepositoryName:             &repo.Name,
		DestinationCommitSpecifier: &pull.BaseBranch,
		SourceCommitSpecifier:      &pull.HeadBranch,
		MergeOption:                &mergeOption,
	})
	if err != nil {
		return nil, err
	}

	mergeCommit := mergeOutput.CommitId
	files := []string{}

	var nextToken *string
	for {
		diffOutput, err := c.Client.GetDifferences(&codecommit.GetDifferencesInput{
			AfterCommitSpecifier: mergeCommit,
			NextToken:            nextToken,
		})
		if err != nil {
			return nil, err
		}
		for _, diff := range diffOutput.Differences {
			files = append(files, *diff.AfterBlob.Path)
			if *diff.AfterBlob.Path != *diff.BeforeBlob.Path {
				files = append(files, *diff.BeforeBlob.Path)
			}
		}
		nextToken = diffOutput.NextToken
		if nextToken == nil {
			break
		}
	}

	return files, nil
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
			return errors.Wrapf(err, "minimize comment %s", *comment.CommentId)
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
	// API possibilities
	// https://github.blog/2012-09-04-commit-status-api/
	// UpdatePullRequestApprovalState

	pullRequestId := strconv.Itoa(pull.Num)
	pullRequest, err := c.Client.GetPullRequest(&codecommit.GetPullRequestInput{
		PullRequestId: &pullRequestId,
	})
	if err != nil {
		return err
	}

	revisionId := *pullRequest.PullRequest.RevisionId

	ccState := "error"
	switch state {
	case models.PendingCommitStatus:
	case models.FailedCommitStatus:
		ccState = "REVOKED"
	case models.SuccessCommitStatus:
		ccState = "APPROVED"
	}
	_, err = c.Client.UpdatePullRequestApprovalState(&codecommit.UpdatePullRequestApprovalStateInput{
		ApprovalState: &ccState,
		PullRequestId: &pullRequestId,
		RevisionId:    &revisionId,
	})
	if err != nil {
		return err
	}

	baseCommitId := *pullRequest.PullRequest.PullRequestTargets[0].MergeBase
	prBranchTipId := *pullRequest.PullRequest.PullRequestTargets[0].SourceCommit
	comment := fmt.Sprintf("Atlantis set pull status approval to : %s", ccState)

	_, err = c.Client.PostCommentForPullRequest(&codecommit.PostCommentForPullRequestInput{
		RepositoryName: &repo.Name,
		PullRequestId:  &pullRequestId,
		BeforeCommitId: &baseCommitId,
		AfterCommitId:  &prBranchTipId,
		Content:        &comment,
	})
	return err
}

func (c *CodeCommitClient) MergePull(pull models.PullRequest) error {
	// API : MergePullRequestBy[ThreeWay|FastForward|Squash]
	// Unlike the GitHub API

	conflictDetail := codecommit.ConflictDetailLevelTypeEnumLineLevel
	pullRequestId := strconv.Itoa(pull.Num)
	repoName := pull.BaseRepo.Name
	commitMessage := common.AutomergeCommitMsg

	_, err := c.Client.MergePullRequestByThreeWay(&codecommit.MergePullRequestByThreeWayInput{
		RepositoryName:      &repoName,
		PullRequestId:       &pullRequestId,
		ConflictDetailLevel: &conflictDetail,
		CommitMessage:       &commitMessage,
	})

	return err
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

// func (c *CodeCommitClient) ContributeConfigFlags(
// 	pstringFlags map[string]cmd.StringFlag,
// 	pintFlags map[string]cmd.IntFlag,
// 	pboolFlags map[string]cmd.BoolFlag,
// ) {

// 	for name, value := range stringFlags {
// 		pstringFlags[name] = value
// 	}

// 	for name, value := range intFlags {
// 		pintFlags[name] = value
// 	}

// 	for name, value := range boolFlags {
// 		pboolFlags[name] = value
// 	}

// }
