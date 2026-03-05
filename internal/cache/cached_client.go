package cache

import (
	"context"
	"encoding/json"
	"log"
	"time"

	gh "github.com/swibrow/github-actions-tui/internal/github"
)

// TTLs for each entity type.
const (
	TTLWorkflows    = 1 * time.Hour
	TTLJobs         = 7 * 24 * time.Hour
	TTLJobLogs      = 30 * 24 * time.Hour
	TTLWorkflowYAML = 24 * time.Hour
	TTLCompletedRun = 7 * 24 * time.Hour
)

// CachedClient wraps a GitHubClient and caches responses in a SQLite Store.
type CachedClient struct {
	inner gh.GitHubClient
	store *Store
	owner string
	repo  string
}

// NewCachedClient returns a CachedClient wrapping inner with the given store.
func NewCachedClient(inner gh.GitHubClient, store *Store, owner, repo string) *CachedClient {
	return &CachedClient{inner: inner, store: store, owner: owner, repo: repo}
}

// FetchWorkflows checks cache first, falls back to API.
func (c *CachedClient) FetchWorkflows(ctx context.Context) ([]gh.Workflow, error) {
	if data, ok := c.store.GetWorkflows(c.owner, c.repo); ok {
		var wfs []gh.Workflow
		if err := json.Unmarshal([]byte(data), &wfs); err == nil {
			log.Printf("cache hit: workflows for %s/%s", c.owner, c.repo)
			return wfs, nil
		}
	}

	log.Printf("cache miss: workflows for %s/%s", c.owner, c.repo)
	wfs, err := c.inner.FetchWorkflows(ctx)
	if err != nil {
		return nil, err
	}

	if data, err := json.Marshal(wfs); err == nil {
		c.store.PutWorkflows(c.owner, c.repo, string(data), TTLWorkflows)
	}
	return wfs, nil
}

// FetchRuns always hits the API for real-time freshness.
// Completed runs are cached as a side-effect for downstream use.
func (c *CachedClient) FetchRuns(ctx context.Context, filter gh.RunFilter) ([]gh.WorkflowRun, error) {
	runs, err := c.inner.FetchRuns(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Cache completed runs as side-effect
	for _, r := range runs {
		if r.Status == "completed" {
			c.cacheRun(r)
		}
	}
	return runs, nil
}

// FetchRunsForWorkflow always hits the API.
// Completed runs are cached as a side-effect.
func (c *CachedClient) FetchRunsForWorkflow(ctx context.Context, workflowID int64, count int) ([]gh.WorkflowRun, error) {
	runs, err := c.inner.FetchRunsForWorkflow(ctx, workflowID, count)
	if err != nil {
		return nil, err
	}

	for _, r := range runs {
		if r.Status == "completed" {
			c.cacheRun(r)
		}
	}
	return runs, nil
}

// FetchJobs checks cache first (only if all jobs in the run were completed when cached).
func (c *CachedClient) FetchJobs(ctx context.Context, runID int64) ([]gh.WorkflowJob, error) {
	if data, ok := c.store.GetJobs(c.owner, c.repo, runID); ok {
		jobs, err := UnmarshalJobs([]byte(data))
		if err == nil {
			log.Printf("cache hit: jobs for run %d", runID)
			return jobs, nil
		}
	}

	log.Printf("cache miss: jobs for run %d", runID)
	jobs, err := c.inner.FetchJobs(ctx, runID)
	if err != nil {
		return nil, err
	}

	// Only cache if ALL jobs are completed
	if allJobsCompleted(jobs) {
		if data, err := MarshalJobs(jobs); err == nil {
			c.store.PutJobs(c.owner, c.repo, runID, string(data), TTLJobs)
		}
	}
	return jobs, nil
}

// FetchJobsForAttempt passes through to the inner client (no caching for specific attempts).
func (c *CachedClient) FetchJobsForAttempt(ctx context.Context, runID int64, attempt int) ([]gh.WorkflowJob, error) {
	return c.inner.FetchJobsForAttempt(ctx, runID, attempt)
}

// FetchJobLogs checks cache first — biggest win since logs are immutable once completed.
func (c *CachedClient) FetchJobLogs(ctx context.Context, jobID int64) (string, error) {
	if content, ok := c.store.GetJobLogs(c.owner, c.repo, jobID); ok {
		log.Printf("cache hit: logs for job %d", jobID)
		return content, nil
	}

	log.Printf("cache miss: logs for job %d", jobID)
	content, err := c.inner.FetchJobLogs(ctx, jobID)
	if err != nil {
		return "", err
	}

	c.store.PutJobLogs(c.owner, c.repo, jobID, content, TTLJobLogs)
	return content, nil
}

// FetchWorkflowYAML checks cache first.
func (c *CachedClient) FetchWorkflowYAML(ctx context.Context, path string) (map[string][]string, error) {
	if data, ok := c.store.GetWorkflowYAML(c.owner, c.repo, path); ok {
		var deps map[string][]string
		if err := json.Unmarshal([]byte(data), &deps); err == nil {
			log.Printf("cache hit: workflow yaml %s", path)
			return deps, nil
		}
	}

	log.Printf("cache miss: workflow yaml %s", path)
	deps, err := c.inner.FetchWorkflowYAML(ctx, path)
	if err != nil {
		return nil, err
	}

	if data, err := json.Marshal(deps); err == nil {
		c.store.PutWorkflowYAML(c.owner, c.repo, path, string(data), TTLWorkflowYAML)
	}
	return deps, nil
}

func (c *CachedClient) SwitchRepo(owner, repo string) {
	c.owner = owner
	c.repo = repo
	c.inner.SwitchRepo(owner, repo)
}

func (c *CachedClient) ListUserRepos(ctx context.Context) ([]gh.Repository, error) {
	return c.inner.ListUserRepos(ctx)
}

func (c *CachedClient) ListUserOrgs(ctx context.Context) ([]string, error) {
	return c.inner.ListUserOrgs(ctx)
}

func (c *CachedClient) ListOrgRepos(ctx context.Context, org string) ([]gh.Repository, error) {
	return c.inner.ListOrgRepos(ctx, org)
}

func (c *CachedClient) SearchRepos(ctx context.Context, query string) ([]gh.Repository, error) {
	return c.inner.SearchRepos(ctx, query)
}

func (c *CachedClient) FetchWorkflowFileContent(ctx context.Context, path string) (string, error) {
	return c.inner.FetchWorkflowFileContent(ctx, path)
}

func (c *CachedClient) RerunWorkflow(ctx context.Context, runID int64) error {
	return c.inner.RerunWorkflow(ctx, runID)
}

func (c *CachedClient) TriggerWorkflow(ctx context.Context, workflowID int64, ref string, inputs map[string]interface{}) error {
	return c.inner.TriggerWorkflow(ctx, workflowID, ref, inputs)
}

func (c *CachedClient) FetchWorkflowInputs(ctx context.Context, path string) ([]gh.WorkflowInput, error) {
	return c.inner.FetchWorkflowInputs(ctx, path)
}

// --- Helpers ---

func (c *CachedClient) cacheRun(r gh.WorkflowRun) {
	data, err := MarshalRuns([]gh.WorkflowRun{r})
	if err != nil {
		return
	}
	c.store.PutRun(c.owner, c.repo, r.ID, r.Status, string(data), TTLCompletedRun)
}

func allJobsCompleted(jobs []gh.WorkflowJob) bool {
	if len(jobs) == 0 {
		return false
	}
	for _, j := range jobs {
		if j.Status != "completed" {
			return false
		}
	}
	return true
}

// --- Serialization helpers for time.Duration fields ---

// runJSON is a JSON-friendly representation of WorkflowRun.
type runJSON struct {
	ID           int64     `json:"id"`
	WorkflowID   int64     `json:"workflow_id"`
	Number       int       `json:"number"`
	RunAttempt   int       `json:"run_attempt"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	Branch       string    `json:"branch"`
	HeadSHA      string    `json:"head_sha"`
	Event        string    `json:"event"`
	Actor        string    `json:"actor"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	RunStartedAt time.Time `json:"run_started_at"`
	DurationNs   int64     `json:"duration_ns"`
}

// MarshalRuns serializes runs with Duration as nanoseconds.
func MarshalRuns(runs []gh.WorkflowRun) ([]byte, error) {
	jruns := make([]runJSON, len(runs))
	for i, r := range runs {
		jruns[i] = runJSON{
			ID:           r.ID,
			WorkflowID:   r.WorkflowID,
			Number:       r.Number,
			RunAttempt:   r.RunAttempt,
			Name:         r.Name,
			Status:       r.Status,
			Conclusion:   r.Conclusion,
			Branch:       r.Branch,
			HeadSHA:      r.HeadSHA,
			Event:        r.Event,
			Actor:        r.Actor,
			CreatedAt:    r.CreatedAt,
			UpdatedAt:    r.UpdatedAt,
			RunStartedAt: r.RunStartedAt,
			DurationNs:   int64(r.Duration),
		}
	}
	return json.Marshal(jruns)
}

// UnmarshalRuns deserializes runs, converting nanoseconds back to Duration.
func UnmarshalRuns(data []byte) ([]gh.WorkflowRun, error) {
	var jruns []runJSON
	if err := json.Unmarshal(data, &jruns); err != nil {
		return nil, err
	}
	runs := make([]gh.WorkflowRun, len(jruns))
	for i, jr := range jruns {
		runs[i] = gh.WorkflowRun{
			ID:           jr.ID,
			WorkflowID:   jr.WorkflowID,
			Number:       jr.Number,
			RunAttempt:   jr.RunAttempt,
			Name:         jr.Name,
			Status:       jr.Status,
			Conclusion:   jr.Conclusion,
			Branch:       jr.Branch,
			HeadSHA:      jr.HeadSHA,
			Event:        jr.Event,
			Actor:        jr.Actor,
			CreatedAt:    jr.CreatedAt,
			UpdatedAt:    jr.UpdatedAt,
			RunStartedAt: jr.RunStartedAt,
			Duration:     time.Duration(jr.DurationNs),
		}
	}
	return runs, nil
}

// jobJSON is a JSON-friendly representation of WorkflowJob.
type jobJSON struct {
	ID         int64         `json:"id"`
	RunID      int64         `json:"run_id"`
	Name       string        `json:"name"`
	Status     string        `json:"status"`
	Conclusion string        `json:"conclusion"`
	StartedAt  time.Time     `json:"started_at"`
	DurationNs int64         `json:"duration_ns"`
	Steps      []gh.JobStep  `json:"steps"`
}

// MarshalJobs serializes jobs with Duration as nanoseconds.
func MarshalJobs(jobs []gh.WorkflowJob) ([]byte, error) {
	jjobs := make([]jobJSON, len(jobs))
	for i, j := range jobs {
		jjobs[i] = jobJSON{
			ID:         j.ID,
			RunID:      j.RunID,
			Name:       j.Name,
			Status:     j.Status,
			Conclusion: j.Conclusion,
			StartedAt:  j.StartedAt,
			DurationNs: int64(j.Duration),
			Steps:      j.Steps,
		}
	}
	return json.Marshal(jjobs)
}

// UnmarshalJobs deserializes jobs, converting nanoseconds back to Duration.
func UnmarshalJobs(data []byte) ([]gh.WorkflowJob, error) {
	var jjobs []jobJSON
	if err := json.Unmarshal(data, &jjobs); err != nil {
		return nil, err
	}
	jobs := make([]gh.WorkflowJob, len(jjobs))
	for i, jj := range jjobs {
		jobs[i] = gh.WorkflowJob{
			ID:         jj.ID,
			RunID:      jj.RunID,
			Name:       jj.Name,
			Status:     jj.Status,
			Conclusion: jj.Conclusion,
			StartedAt:  jj.StartedAt,
			Duration:   time.Duration(jj.DurationNs),
			Steps:      jj.Steps,
		}
	}
	return jobs, nil
}
