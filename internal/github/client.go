package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	gh "github.com/cli/go-gh/v2/pkg/auth"
	ghrepo "github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"
)

type Workflow struct {
	ID   int64
	Name string
	Path string // e.g. ".github/workflows/ci.yml"
}

type WorkflowRun struct {
	ID           int64
	WorkflowID   int64
	Number       int
	RunAttempt   int
	Name         string
	Status       string
	Conclusion   string
	Branch       string
	HeadSHA      string
	Event        string
	Actor        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	RunStartedAt time.Time
	Duration     time.Duration
}

type WorkflowJob struct {
	ID         int64
	RunID      int64
	RunAttempt int
	Name       string
	Status     string
	Conclusion string
	StartedAt  time.Time
	Duration   time.Duration
	Steps      []JobStep
}

type JobStep struct {
	Name       string
	Status     string
	Conclusion string
	Number     int64
}

type Repository struct {
	Owner       string
	Name        string
	FullName    string
	Description string
	Private     bool
}

type RunFilter struct {
	WorkflowID int64
	Branch     string
	Actor      string
	Status     string
	Event      string
}

// GitHubClient is the interface satisfied by *Client and used by the UI layer.
type GitHubClient interface {
	FetchWorkflows(ctx context.Context) ([]Workflow, error)
	FetchRuns(ctx context.Context, filter RunFilter) ([]WorkflowRun, error)
	FetchJobs(ctx context.Context, runID int64) ([]WorkflowJob, error)
	FetchJobsForAttempt(ctx context.Context, runID int64, attempt int) ([]WorkflowJob, error)
	FetchJobLogs(ctx context.Context, jobID int64) (string, error)
	FetchRunsForWorkflow(ctx context.Context, workflowID int64, count int) ([]WorkflowRun, error)
	FetchWorkflowYAML(ctx context.Context, path string) (map[string][]string, error)
	SwitchRepo(owner, repo string)
	ListUserRepos(ctx context.Context) ([]Repository, error)
	ListUserOrgs(ctx context.Context) ([]string, error)
	ListOrgRepos(ctx context.Context, org string) ([]Repository, error)
	SearchRepos(ctx context.Context, query string) ([]Repository, error)
}

type Client struct {
	gh    *github.Client
	owner string
	repo  string
}

func DetectRepo() (owner, repo string, err error) {
	r, err := ghrepo.Current()
	if err != nil {
		return "", "", fmt.Errorf("detecting repo: %w (are you in a git repo with a GitHub remote?)", err)
	}
	return r.Owner, r.Name, nil
}

func NewClient(owner, repo string) (*Client, error) {
	token, _ := gh.TokenForHost("github.com")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("no GitHub token found: run 'gh auth login' or set GH_TOKEN/GITHUB_TOKEN")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(httpClient)

	return &Client{gh: client, owner: owner, repo: repo}, nil
}

func (c *Client) FetchWorkflows(ctx context.Context) ([]Workflow, error) {
	opts := &github.ListOptions{PerPage: 100}
	result, _, err := c.gh.Actions.ListWorkflows(ctx, c.owner, c.repo, opts)
	if err != nil {
		return nil, fmt.Errorf("fetching workflows: %w", err)
	}

	workflows := make([]Workflow, 0, len(result.Workflows))
	for _, w := range result.Workflows {
		workflows = append(workflows, Workflow{
			ID:   w.GetID(),
			Name: w.GetName(),
			Path: w.GetPath(),
		})
	}
	return workflows, nil
}

func (c *Client) FetchRuns(ctx context.Context, filter RunFilter) ([]WorkflowRun, error) {
	opts := &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{PerPage: 30},
	}
	if filter.Branch != "" {
		opts.Branch = filter.Branch
	}
	if filter.Actor != "" {
		opts.Actor = filter.Actor
	}
	if filter.Status != "" {
		opts.Status = filter.Status
	}
	if filter.Event != "" {
		opts.Event = filter.Event
	}

	var result *github.WorkflowRuns
	var err error

	if filter.WorkflowID > 0 {
		result, _, err = c.gh.Actions.ListWorkflowRunsByID(ctx, c.owner, c.repo, filter.WorkflowID, opts)
	} else {
		result, _, err = c.gh.Actions.ListRepositoryWorkflowRuns(ctx, c.owner, c.repo, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("fetching runs: %w", err)
	}

	runs := make([]WorkflowRun, 0, len(result.WorkflowRuns))
	for _, r := range result.WorkflowRuns {
		run := WorkflowRun{
			ID:         r.GetID(),
			WorkflowID: r.GetWorkflowID(),
			Number:     r.GetRunNumber(),
			RunAttempt: r.GetRunAttempt(),
			Name:       r.GetName(),
			Status:     r.GetStatus(),
			Conclusion: r.GetConclusion(),
			Branch:     r.GetHeadBranch(),
			HeadSHA:    r.GetHeadSHA(),
			Event:      r.GetEvent(),
			Actor:      r.GetActor().GetLogin(),
			CreatedAt:  r.GetCreatedAt().Time,
			UpdatedAt:  r.GetUpdatedAt().Time,
		}
		if r.RunStartedAt != nil {
			run.RunStartedAt = r.GetRunStartedAt().Time
		}
		if run.Status == "completed" && !run.RunStartedAt.IsZero() {
			run.Duration = run.UpdatedAt.Sub(run.RunStartedAt)
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (c *Client) FetchJobs(ctx context.Context, runID int64) ([]WorkflowJob, error) {
	opts := &github.ListWorkflowJobsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	result, _, err := c.gh.Actions.ListWorkflowJobs(ctx, c.owner, c.repo, runID, opts)
	if err != nil {
		return nil, fmt.Errorf("fetching jobs: %w", err)
	}

	jobs := make([]WorkflowJob, 0, len(result.Jobs))
	for _, j := range result.Jobs {
		job := WorkflowJob{
			ID:         j.GetID(),
			RunID:      j.GetRunID(),
			RunAttempt: int(j.GetRunAttempt()),
			Name:       j.GetName(),
			Status:     j.GetStatus(),
			Conclusion: j.GetConclusion(),
		}
		if j.StartedAt != nil {
			job.StartedAt = j.GetStartedAt().Time
		}
		if j.CompletedAt != nil && j.StartedAt != nil {
			job.Duration = j.GetCompletedAt().Time.Sub(j.GetStartedAt().Time)
		}
		for _, s := range j.Steps {
			job.Steps = append(job.Steps, JobStep{
				Name:       s.GetName(),
				Status:     s.GetStatus(),
				Conclusion: s.GetConclusion(),
				Number:     s.GetNumber(),
			})
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (c *Client) FetchJobsForAttempt(ctx context.Context, runID int64, attempt int) ([]WorkflowJob, error) {
	opts := &github.ListOptions{PerPage: 100}
	result, _, err := c.gh.Actions.ListWorkflowJobsAttempt(ctx, c.owner, c.repo, runID, int64(attempt), opts)
	if err != nil {
		return nil, fmt.Errorf("fetching jobs for attempt %d: %w", attempt, err)
	}

	jobs := make([]WorkflowJob, 0, len(result.Jobs))
	for _, j := range result.Jobs {
		job := WorkflowJob{
			ID:         j.GetID(),
			RunID:      j.GetRunID(),
			RunAttempt: int(j.GetRunAttempt()),
			Name:       j.GetName(),
			Status:     j.GetStatus(),
			Conclusion: j.GetConclusion(),
		}
		if j.StartedAt != nil {
			job.StartedAt = j.GetStartedAt().Time
		}
		if j.CompletedAt != nil && j.StartedAt != nil {
			job.Duration = j.GetCompletedAt().Time.Sub(j.GetStartedAt().Time)
		}
		for _, s := range j.Steps {
			job.Steps = append(job.Steps, JobStep{
				Name:       s.GetName(),
				Status:     s.GetStatus(),
				Conclusion: s.GetConclusion(),
				Number:     s.GetNumber(),
			})
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (c *Client) FetchJobLogs(ctx context.Context, jobID int64) (string, error) {
	url, _, err := c.gh.Actions.GetWorkflowJobLogs(ctx, c.owner, c.repo, jobID, 2)
	if err != nil {
		return "", fmt.Errorf("fetching job logs URL: %w", err)
	}

	resp, err := http.Get(url.String())
	if err != nil {
		return "", fmt.Errorf("downloading job logs: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading job logs: %w", err)
	}
	return string(body), nil
}

func (c *Client) FetchRunsForWorkflow(ctx context.Context, workflowID int64, count int) ([]WorkflowRun, error) {
	opts := &github.ListWorkflowRunsOptions{
		ListOptions: github.ListOptions{PerPage: count},
	}
	result, _, err := c.gh.Actions.ListWorkflowRunsByID(ctx, c.owner, c.repo, workflowID, opts)
	if err != nil {
		return nil, fmt.Errorf("fetching runs for workflow: %w", err)
	}

	runs := make([]WorkflowRun, 0, len(result.WorkflowRuns))
	for _, r := range result.WorkflowRuns {
		run := WorkflowRun{
			ID:         r.GetID(),
			WorkflowID: r.GetWorkflowID(),
			Number:     r.GetRunNumber(),
			RunAttempt: r.GetRunAttempt(),
			Name:       r.GetName(),
			Status:     r.GetStatus(),
			Conclusion: r.GetConclusion(),
			Branch:     r.GetHeadBranch(),
			HeadSHA:    r.GetHeadSHA(),
			Event:      r.GetEvent(),
			Actor:      r.GetActor().GetLogin(),
			CreatedAt:  r.GetCreatedAt().Time,
			UpdatedAt:  r.GetUpdatedAt().Time,
		}
		if r.RunStartedAt != nil {
			run.RunStartedAt = r.GetRunStartedAt().Time
		}
		if run.Status == "completed" && !run.RunStartedAt.IsZero() {
			run.Duration = run.UpdatedAt.Sub(run.RunStartedAt)
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (c *Client) FetchFileContent(ctx context.Context, path string) ([]byte, error) {
	fileContent, _, _, err := c.gh.Repositories.GetContents(ctx, c.owner, c.repo, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching file content: %w", err)
	}
	if fileContent == nil {
		return nil, fmt.Errorf("path %s is not a file", path)
	}
	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decoding file content: %w", err)
	}
	return []byte(content), nil
}

func (c *Client) SwitchRepo(owner, repo string) {
	c.owner = owner
	c.repo = repo
}

func (c *Client) ListUserRepos(ctx context.Context) ([]Repository, error) {
	opts := &github.RepositoryListByAuthenticatedUserOptions{
		Sort:        "updated",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	var all []Repository
	for {
		repos, resp, err := c.gh.Repositories.ListByAuthenticatedUser(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("listing user repos: %w", err)
		}
		for _, r := range repos {
			all = append(all, Repository{
				Owner:       r.GetOwner().GetLogin(),
				Name:        r.GetName(),
				FullName:    r.GetFullName(),
				Description: r.GetDescription(),
				Private:     r.GetPrivate(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

func (c *Client) ListUserOrgs(ctx context.Context) ([]string, error) {
	opts := &github.ListOptions{PerPage: 100}
	orgs, _, err := c.gh.Organizations.List(ctx, "", opts)
	if err != nil {
		return nil, fmt.Errorf("listing orgs: %w", err)
	}
	names := make([]string, 0, len(orgs))
	for _, o := range orgs {
		names = append(names, o.GetLogin())
	}
	return names, nil
}

func (c *Client) ListOrgRepos(ctx context.Context, org string) ([]Repository, error) {
	opts := &github.RepositoryListByOrgOptions{
		Sort:        "updated",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	var all []Repository
	for {
		repos, resp, err := c.gh.Repositories.ListByOrg(ctx, org, opts)
		if err != nil {
			return nil, fmt.Errorf("listing org repos: %w", err)
		}
		for _, r := range repos {
			all = append(all, Repository{
				Owner:       r.GetOwner().GetLogin(),
				Name:        r.GetName(),
				FullName:    r.GetFullName(),
				Description: r.GetDescription(),
				Private:     r.GetPrivate(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

func (c *Client) SearchRepos(ctx context.Context, query string) ([]Repository, error) {
	opts := &github.SearchOptions{
		Sort:        "stars",
		ListOptions: github.ListOptions{PerPage: 30},
	}
	result, _, err := c.gh.Search.Repositories(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("searching repos: %w", err)
	}
	repos := make([]Repository, 0, len(result.Repositories))
	for _, r := range result.Repositories {
		repos = append(repos, Repository{
			Owner:       r.GetOwner().GetLogin(),
			Name:        r.GetName(),
			FullName:    r.GetFullName(),
			Description: r.GetDescription(),
			Private:     r.GetPrivate(),
		})
	}
	return repos, nil
}

func HasActiveRuns(runs []WorkflowRun) bool {
	for _, r := range runs {
		if r.Status == "in_progress" || r.Status == "queued" || r.Status == "waiting" || r.Status == "pending" {
			return true
		}
	}
	return false
}
