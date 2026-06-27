package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type workflowRun struct {
	ID         int64
	Status     string
	Conclusion string
	HTMLURL    string
	CreatedAt  time.Time
	HeadBranch string
	Event      string
}

type workflowRunsResponse struct {
	WorkflowRuns []struct {
		ID         int64   `json:"id"`
		Status     string  `json:"status"`
		Conclusion *string `json:"conclusion"`
		HTMLURL    string  `json:"html_url"`
		CreatedAt  string  `json:"created_at"`
		Event      string  `json:"event"`
		HeadBranch string  `json:"head_branch"`
	} `json:"workflow_runs"`
}

func (d *DeployTrigger) WaitGreenWorkflows(ctx context.Context, since time.Time, maxWait time.Duration) (string, error) {
	return d.WaitSlotWorkflows(ctx, since, maxWait)
}

func (d *DeployTrigger) WaitSlotWorkflows(ctx context.Context, since time.Time, maxWait time.Duration) (string, error) {
	return d.waitRepoWorkflows(ctx, since, maxWait)
}

func (d *DeployTrigger) waitRepoWorkflows(ctx context.Context, since time.Time, maxWait time.Duration) (string, error) {
	backend, err := d.backendTarget("")
	if err != nil {
		return "", err
	}
	frontend, err := d.frontendTarget("")
	if err != nil {
		return "", err
	}
	return d.waitRepoWorkflowsForTargets(ctx, []repoTarget{backend, frontend}, since, maxWait)
}

func (d *DeployTrigger) waitRepoWorkflowsForTargets(ctx context.Context, targets []repoTarget, since time.Time, maxWait time.Duration) (string, error) {
	if d.cfg.GitHubToken == "" {
		return "", fmt.Errorf("GITHUB_TOKEN not configured")
	}
	deadline := time.Now().Add(maxWait)
	pending := make(map[string]repoTarget, len(targets))
	for _, t := range targets {
		pending[t.repo] = t
	}

	var done []string
	poll := 10 * time.Second

	for time.Now().Before(deadline) {
		for repo, t := range pending {
			run, err := d.fetchLatestDeployRun(ctx, t.repo, t.gitRef, since)
			if err != nil {
				return "", err
			}
			if run == nil {
				continue
			}
			switch run.Status {
			case "completed":
				if run.Conclusion != "success" {
					c := run.Conclusion
					if c == "" {
						c = "failed"
					}
					return "", fmt.Errorf("%s workflow 未成功（%s） %s", t.repo, c, run.HTMLURL)
				}
				delete(pending, repo)
				done = append(done, fmt.Sprintf("%s run#%d ok (%s)", t.repo, run.ID, run.HeadBranch))
			case "queued", "in_progress", "waiting", "requested", "pending":
			default:
			}
		}
		if len(pending) == 0 {
			return strings.Join(done, "; "), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(poll):
		}
	}

	var waiting []string
	for repo, t := range pending {
		waiting = append(waiting, fmt.Sprintf("%s@%s", repo, t.gitRef))
	}
	return "", fmt.Errorf("等待 GitHub Actions 超时（%s），仍在进行: %s", maxWait, strings.Join(waiting, ", "))
}

func (d *DeployTrigger) fetchLatestDeployRun(ctx context.Context, repo, gitRef string, since time.Time) (*workflowRun, error) {
	return d.fetchLatestRunByEvent(ctx, repo, "workflow_dispatch", since, gitRef)
}

func (d *DeployTrigger) fetchLatestRunByEvent(ctx context.Context, repo, event string, since time.Time, gitRef string) (*workflowRun, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo %q", repo)
	}
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/actions/runs?event=%s&per_page=15",
		parts[0], parts[1], event,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github list runs HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var body workflowRunsResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}

	var best *workflowRun
	for _, wr := range body.WorkflowRuns {
		if wr.Event != event {
			continue
		}
		created, err := time.Parse(time.RFC3339, wr.CreatedAt)
		if err != nil {
			continue
		}
		if created.Before(since) {
			continue
		}
		if gitRef != "" && wr.HeadBranch != "" && wr.HeadBranch != gitRef {
			continue
		}
		conclusion := ""
		if wr.Conclusion != nil {
			conclusion = *wr.Conclusion
		}
		candidate := &workflowRun{
			ID: wr.ID, Status: wr.Status, Conclusion: conclusion,
			HTMLURL: wr.HTMLURL, CreatedAt: created,
			HeadBranch: wr.HeadBranch, Event: wr.Event,
		}
		if best == nil || candidate.CreatedAt.After(best.CreatedAt) {
			best = candidate
		}
	}
	return best, nil
}
