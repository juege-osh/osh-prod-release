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

func (d *DeployTrigger) CancelActiveWorkflows(ctx context.Context, since time.Time) (string, error) {
	backend, err := d.backendTarget("")
	if err != nil {
		return "", err
	}
	frontend, err := d.frontendTarget("")
	if err != nil {
		return "", err
	}

	var parts []string
	for _, t := range []repoTarget{backend, frontend} {
		n, err := d.cancelRepoRuns(ctx, t.repo, t.gitRef, since)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%s cancel err: %v", t.repo, err))
			continue
		}
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s cancelled %d run(s)", t.repo, n))
		}
	}
	if len(parts) == 0 {
		return "no active runs to cancel", nil
	}
	return strings.Join(parts, "; "), nil
}

func (d *DeployTrigger) cancelRepoRuns(ctx context.Context, repo, gitRef string, since time.Time) (int, error) {
	if d.cfg.GitHubToken == "" {
		return 0, fmt.Errorf("GITHUB_TOKEN not configured")
	}
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid repo %q", repo)
	}

	var cancelled int
	for _, status := range []string{"in_progress", "queued", "waiting", "requested", "pending"} {
		url := fmt.Sprintf(
			"https://api.github.com/repos/%s/%s/actions/runs?event=workflow_dispatch&status=%s&per_page=10",
			parts[0], parts[1], status,
		)
		runs, err := d.listRuns(ctx, url)
		if err != nil {
			return cancelled, err
		}
		for _, run := range runs {
			if gitRef != "" && run.HeadBranch != "" && run.HeadBranch != gitRef {
				continue
			}
			if run.CreatedAt.Before(since) {
				continue
			}
			if err := d.cancelRun(ctx, parts[0], parts[1], run.ID); err != nil {
				return cancelled, err
			}
			cancelled++
		}
	}
	return cancelled, nil
}

func (d *DeployTrigger) listRuns(ctx context.Context, url string) ([]workflowRun, error) {
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

	var out []workflowRun
	for _, wr := range body.WorkflowRuns {
		created, err := time.Parse(time.RFC3339, wr.CreatedAt)
		if err != nil {
			continue
		}
		conclusion := ""
		if wr.Conclusion != nil {
			conclusion = *wr.Conclusion
		}
		out = append(out, workflowRun{
			ID: wr.ID, Status: wr.Status, Conclusion: conclusion,
			HTMLURL: wr.HTMLURL, CreatedAt: created, HeadBranch: wr.HeadBranch, Event: wr.Event,
		})
	}
	return out, nil
}

func (d *DeployTrigger) cancelRun(ctx context.Context, owner, repo string, runID int64) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/runs/%d/cancel", owner, repo, runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("github cancel run HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}
