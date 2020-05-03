package vcs_test

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/service/codecommit"
	"github.com/aws/aws-sdk-go/service/codecommit/codecommitiface"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/vcs"
	. "github.com/runatlantis/atlantis/testing"
)

type postCommentMock struct {
	codecommitiface.CodeCommitAPI
	PageCount int
}

func (m *postCommentMock) GetPullRequest(input *codecommit.GetPullRequestInput) (*codecommit.GetPullRequestOutput, error) {
	baseRevision := "base-revision"
	prTipRevision := "pr-tip-revision"
	return &codecommit.GetPullRequestOutput{
		PullRequest: &codecommit.PullRequest{
			PullRequestTargets: []*codecommit.PullRequestTarget{
				{
					MergeBase:    &baseRevision,
					SourceCommit: &prTipRevision,
				},
			},
		},
	}, nil
}

func (m *postCommentMock) PostCommentForPullRequest(input *codecommit.PostCommentForPullRequestInput) (*codecommit.PostCommentForPullRequestOutput, error) {
	m.PageCount++
	return nil, nil
}

func TestCodeCommitClient_PaginatesComments(t *testing.T) {
	mock := &postCommentMock{}
	client := &vcs.CodeCommitClient{
		Client: mock,
	}
	repo := models.Repo{
		Name: "atlantis-test",
	}
	pullNum := 1
	// limit is 1,000, so 20 chars * 100 should split across *3* pages (because of header/footer)
	comment := strings.Repeat("99 bottles of beer!\n", 100)
	err := client.CreateComment(repo, pullNum, comment)
	Ok(t, err)
	Equals(t, 3, mock.PageCount)
}

type hideCommentsMock struct {
	codecommitiface.CodeCommitAPI
	pagesRemaining  int
	commentsDeleted int
}

func (m *hideCommentsMock) GetCommentsForPullRequest(input *codecommit.GetCommentsForPullRequestInput) (*codecommit.GetCommentsForPullRequestOutput, error) {
	atlantisArn := "arn:aws:iam::account-id:user/atlantis"
	userArn := "arn:aws:iam::account-id:user/fred.bloggs"
	commentId := "a-comment-from-atlantis"
	planContent := "a cunning plan\nis never revealed\ntoo soon\n"
	output := codecommit.GetCommentsForPullRequestOutput{
		CommentsForPullRequestData: []*codecommit.CommentsForPullRequest{
			{
				PullRequestId: input.PullRequestId,
				Comments: []*codecommit.Comment{
					{
						AuthorArn: &userArn,
					},
					{
						AuthorArn: &atlantisArn,
						CommentId: &commentId,
						Content:   &planContent,
					},
				},
			},
		},
	}
	if m.pagesRemaining > 0 {
		m.pagesRemaining--
		nextToken := fmt.Sprintf("next-token-%d", m.pagesRemaining)
		output.NextToken = &nextToken
	}

	return &output, nil
}

func (m *hideCommentsMock) DeleteCommentContent(*codecommit.DeleteCommentContentInput) (*codecommit.DeleteCommentContentOutput, error) {
	m.commentsDeleted++
	return nil, nil
}

func TestCodeCommitClient_HideOldComments(t *testing.T) {
	mock := hideCommentsMock{
		pagesRemaining: 2,
	}
	client := &vcs.CodeCommitClient{
		Client:  &mock,
		UserArn: "arn:aws:iam::account-id:user/atlantis",
	}
	repo := models.Repo{}
	pullNum := 1
	err := client.HidePrevPlanComments(repo, pullNum)
	Ok(t, err)
	Equals(t, 3, mock.commentsDeleted)
}

type updateStatusMock struct {
	codecommitiface.CodeCommitAPI
}

func (m *updateStatusMock) GetPullRequest(input *codecommit.GetPullRequestInput) (*codecommit.GetPullRequestOutput, error) {
	revisionId := "a-revision-of-splendour"
	mergeBase := "refs/heads/master"
	sourceCommit := "refs/heads/pr-01"
	return &codecommit.GetPullRequestOutput{
		PullRequest: &codecommit.PullRequest{
			RevisionId: &revisionId,
			PullRequestTargets: []*codecommit.PullRequestTarget{
				{
					MergeBase:    &mergeBase,
					SourceCommit: &sourceCommit,
				},
			},
		},
	}, nil
}

func (m *updateStatusMock) UpdatePullRequestApprovalState(input *codecommit.UpdatePullRequestApprovalStateInput) (*codecommit.UpdatePullRequestApprovalStateOutput, error) {
	return &codecommit.UpdatePullRequestApprovalStateOutput{}, nil
}

func (m *updateStatusMock) PostCommentForPullRequest(input *codecommit.PostCommentForPullRequestInput) (*codecommit.PostCommentForPullRequestOutput, error) {
	return &codecommit.PostCommentForPullRequestOutput{}, nil
}

func TestCodeCommitClient_UpdateStatus(t *testing.T) {
	client := vcs.CodeCommitClient{Client: &updateStatusMock{}}
	err := client.UpdateStatus(
		models.Repo{},
		models.PullRequest{},
		models.SuccessCommitStatus,
		"source is ignored",
		"description is ignored",
		"url is ignored",
	)
	Ok(t, err)
}

type pullIsApprovedMock struct {
	codecommitiface.CodeCommitAPI
	RepoName             string
	PullRequestId        int
	Approved             bool
	calledGetPullRequest bool
	calledEvaluate       bool
}

func (m *pullIsApprovedMock) GetPullRequest(input *codecommit.GetPullRequestInput) (*codecommit.GetPullRequestOutput, error) {
	m.calledGetPullRequest = true
	pullRequestId := strconv.Itoa(m.PullRequestId)
	revisionId := "mac-and-cheese"
	return &codecommit.GetPullRequestOutput{
		PullRequest: &codecommit.PullRequest{
			PullRequestId: &pullRequestId,
			RevisionId:    &revisionId,
		},
	}, nil
}

func (m *pullIsApprovedMock) EvaluatePullRequestApprovalRules(input *codecommit.EvaluatePullRequestApprovalRulesInput) (*codecommit.EvaluatePullRequestApprovalRulesOutput, error) {
	m.calledEvaluate = true
	return &codecommit.EvaluatePullRequestApprovalRulesOutput{
		Evaluation: &codecommit.Evaluation{
			Approved: &m.Approved,
		},
	}, nil
}

func (m *pullIsApprovedMock) Used() bool {
	return m.calledEvaluate && m.calledGetPullRequest
}

func TestCodeCommitClient_PullIsApproved(t *testing.T) {
	// Approved
	mock := &pullIsApprovedMock{
		Approved: true,
	}
	client := vcs.CodeCommitClient{
		Client: mock,
	}
	approved, err := client.PullIsApproved(models.Repo{}, models.PullRequest{})
	Ok(t, err)
	Equals(t, true, approved)
	Equals(t, true, mock.Used())
	// Not Approved
	mock = &pullIsApprovedMock{
		Approved: false,
	}
	client = vcs.CodeCommitClient{
		Client: mock,
	}
	approved, err = client.PullIsApproved(models.Repo{}, models.PullRequest{})
	Ok(t, err)
	Equals(t, false, approved)
	Equals(t, true, mock.Used())
}

type pullMergeableMock struct {
	codecommitiface.CodeCommitAPI
}

func (m *pullMergeableMock) GetMergeConflicts(input *codecommit.GetMergeConflictsInput) (*codecommit.GetMergeConflictsOutput, error) {
	isTrue := true
	isFalse := false
	switch *input.SourceCommitSpecifier {
	case "refs/heads/pr-01":
		return &codecommit.GetMergeConflictsOutput{
			Mergeable: &isTrue,
		}, nil
	case "refs/heads/pr-02":
		return &codecommit.GetMergeConflictsOutput{
			Mergeable: &isFalse,
		}, nil
	default:
		return nil, errors.New("Unknown branch")
	}
}

func TestCodeCommit_PullIsMergeable(t *testing.T) {
	mock := &pullMergeableMock{}
	client := vcs.CodeCommitClient{
		Client: mock,
	}
	// PR1 can merge
	mergeable, err := client.PullIsMergeable(
		models.Repo{},
		models.PullRequest{
			BaseRepo: models.Repo{
				Name: "atlantis-test",
			},
			BaseBranch: "refs/heads/master",
			HeadBranch: "refs/heads/pr-01",
		},
	)
	Ok(t, err)
	Equals(t, true, mergeable)
	// PR2 can't
	mergeable, err = client.PullIsMergeable(
		models.Repo{},
		models.PullRequest{
			BaseRepo: models.Repo{
				Name: "atlantis-test",
			},
			BaseBranch: "refs/heads/master",
			HeadBranch: "refs/heads/pr-02",
		},
	)
	Ok(t, err)
	Equals(t, false, mergeable)
}

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
