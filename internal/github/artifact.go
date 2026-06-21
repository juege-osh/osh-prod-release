package github

import (
	"context"
	"fmt"

	"github.com/juege/osh-prod-release/internal/config"
)

// ArtifactService resolves commit to deployable artifact (P1: stub / local path).
type ArtifactService struct {
	cfg *config.Config
}

func New(cfg *config.Config) *ArtifactService {
	return &ArtifactService{cfg: cfg}
}

type Artifact struct {
	CommitSHA string `json:"commit_sha"`
	Repo      string `json:"repo"`
	Version   string `json:"version"`
	Checksum  string `json:"checksum"`
	Source    string `json:"source"`
}

func (s *ArtifactService) Resolve(ctx context.Context, repo, commitSHA string) (*Artifact, error) {
	_ = ctx
	if commitSHA == "" {
		return nil, fmt.Errorf("commit_sha required")
	}
	if repo == "" {
		repo = s.cfg.GitHubRepo
	}
	// P1: real GitHub clone/build in P2; now register artifact metadata only.
	return &Artifact{
		CommitSHA: commitSHA,
		Repo:      repo,
		Version:   commitSHA[:min(12, len(commitSHA))],
		Checksum:  "pending-build",
		Source:    "github-stub",
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
