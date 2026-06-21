package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/juege/osh-prod-release/internal/config"
)

const slotWorkflowFile = "deploy-149.yml"

// DeployTrigger dispatches approval-gated 149 deploy workflows (green or blue slot).
type DeployTrigger struct {
	cfg *config.Config
}

func NewDeployTrigger(cfg *config.Config) *DeployTrigger {
	return &DeployTrigger{cfg: cfg}
}

type dispatchResult struct {
	Repo        string `json:"repo"`
	Workflow    string `json:"workflow"`
	DispatchRef string `json:"dispatch_ref"`
	GitRef      string `json:"git_ref"`
	Slot        string `json:"slot"`
}

type repoTarget struct {
	repo        string
	dispatchRef string
	gitRef      string
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func (d *DeployTrigger) backendTarget(overrideGitRef string) (repoTarget, error) {
	repo := strings.TrimSpace(d.cfg.GitHubBackendRepo)
	if repo == "" {
		return repoTarget{}, fmt.Errorf("GITHUB_BACKEND_REPO not configured")
	}
	gitRef := firstNonEmpty(overrideGitRef, d.cfg.GitHubBackendGitRef, d.cfg.GitHubBackendDispatchRef, d.cfg.GitHubDispatchRef)
	dispatchRef := firstNonEmpty(d.cfg.GitHubBackendDispatchRef, gitRef, d.cfg.GitHubDispatchRef)
	if gitRef == "" || dispatchRef == "" {
		return repoTarget{}, fmt.Errorf("backend git ref not configured (set GITHUB_BACKEND_GIT_REF or GITHUB_BACKEND_DISPATCH_REF)")
	}
	return repoTarget{repo: repo, dispatchRef: dispatchRef, gitRef: gitRef}, nil
}

func (d *DeployTrigger) frontendTarget(overrideGitRef string) (repoTarget, error) {
	repo := strings.TrimSpace(d.cfg.GitHubFrontendRepo)
	if repo == "" {
		return repoTarget{}, fmt.Errorf("GITHUB_FRONTEND_REPO not configured")
	}
	gitRef := firstNonEmpty(overrideGitRef, d.cfg.GitHubFrontendGitRef, d.cfg.GitHubFrontendDispatchRef, d.cfg.GitHubDispatchRef)
	dispatchRef := firstNonEmpty(d.cfg.GitHubFrontendDispatchRef, gitRef, d.cfg.GitHubDispatchRef)
	if gitRef == "" || dispatchRef == "" {
		return repoTarget{}, fmt.Errorf("frontend git ref not configured (set GITHUB_FRONTEND_GIT_REF or GITHUB_FRONTEND_DISPATCH_REF)")
	}
	return repoTarget{repo: repo, dispatchRef: dispatchRef, gitRef: gitRef}, nil
}

// TriggerSlot149 starts backend + frontend deploy-149.yml for green or blue.
// Pass empty override refs to use config.env per-repo branch names.
func (d *DeployTrigger) TriggerSlot149(ctx context.Context, backendGitRef, frontendGitRef, releaseID, slot string) (string, error) {
	slot = strings.ToLower(strings.TrimSpace(slot))
	if slot != "green" && slot != "blue" {
		return "", fmt.Errorf("slot must be green or blue")
	}
	if d.cfg.GitHubToken == "" {
		return "", fmt.Errorf("GITHUB_TOKEN not configured")
	}

	backend, err := d.backendTarget(backendGitRef)
	if err != nil {
		return "", err
	}
	frontend, err := d.frontendTarget(frontendGitRef)
	if err != nil {
		return "", err
	}

	targets := []repoTarget{backend, frontend}
	var results []dispatchResult
	for _, t := range targets {
		if err := d.dispatchWorkflow(ctx, t.repo, slotWorkflowFile, t.dispatchRef, t.gitRef, releaseID, slot); err != nil {
			return "", fmt.Errorf("%s: %w", t.repo, err)
		}
		results = append(results, dispatchResult{
			Repo: t.repo, Workflow: slotWorkflowFile,
			DispatchRef: t.dispatchRef, GitRef: t.gitRef, Slot: slot,
		})
	}
	b, _ := json.Marshal(results)
	return string(b), nil
}

// TriggerGreen149 is an alias for TriggerSlot149(..., "green").
func (d *DeployTrigger) TriggerGreen149(ctx context.Context, backendGitRef, frontendGitRef, releaseID string) (string, error) {
	return d.TriggerSlot149(ctx, backendGitRef, frontendGitRef, releaseID, "green")
}

func (d *DeployTrigger) dispatchWorkflow(ctx context.Context, repo, workflowFile, ref, gitRef, releaseID, slot string) error {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo %q", repo)
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/dispatches",
		parts[0], parts[1], workflowFile)

	body := map[string]any{
		"ref": ref,
		"inputs": map[string]string{
			"git_ref":    gitRef,
			"release_id": releaseID,
			"slot":       slot,
		},
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github dispatch HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}
