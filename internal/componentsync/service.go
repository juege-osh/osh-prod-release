package componentsync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/juege/osh-prod-release/internal/ssh"
	"github.com/juege/osh-prod-release/internal/store"
	"github.com/juege/osh-prod-release/internal/traffic"
)

type JobStatus string

const (
	StatusRunning JobStatus = "running"
	StatusSuccess JobStatus = "success"
	StatusFailed  JobStatus = "failed"
)

type Job struct {
	ID        string    `json:"id"`
	Direction string    `json:"direction"`
	Status    JobStatus `json:"status"`
	Message   string    `json:"message,omitempty"`
	Output    string    `json:"output,omitempty"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

type ActiveResponse struct {
	Busy bool `json:"busy"`
	Job  *Job `json:"job,omitempty"`
}

type Service struct {
	store   *store.Store
	ssh     *ssh.Client
	traffic *traffic.Service

	mu     sync.RWMutex
	active *Job
}

func New(st *store.Store, sshClient *ssh.Client, trafficSvc *traffic.Service) *Service {
	return &Service{store: st, ssh: sshClient, traffic: trafficSvc}
}

func (s *Service) Active() ActiveResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.active == nil {
		return ActiveResponse{Busy: false}
	}
	j := *s.active
	return ActiveResponse{Busy: j.Status == StatusRunning, Job: &j}
}

func (s *Service) StartBlueToGreenAll(ctx context.Context, actor, reason string) (*Job, error) {
	if s.traffic == nil {
		return nil, fmt.Errorf("traffic service not configured")
	}
	if err := s.traffic.RequireProductionBlue(ctx); err != nil {
		return nil, err
	}
	activeRel, err := s.store.GetActiveDeployingRelease(ctx)
	if err != nil {
		return nil, err
	}
	if activeRel != nil {
		return nil, fmt.Errorf("发布单「%s」正在部署中，请等待完成后再同步组件", activeRel.Title)
	}
	s.mu.RLock()
	if s.active != nil && s.active.Status == StatusRunning {
		s.mu.RUnlock()
		return nil, fmt.Errorf("蓝到绿组件同步正在执行中，请等待完成")
	}
	s.mu.RUnlock()

	job := &Job{
		ID:        uuid.New().String()[:12],
		Direction: "blue-to-green",
		Status:    StatusRunning,
		Message:   "正在执行蓝→绿所有组件增量同步…",
		Actor:     actor,
		Reason:    reason,
		StartedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.active = job
	s.mu.Unlock()

	go s.runBlueToGreenAll(job.ID, actor, reason)
	return s.snapshotJob(job.ID), nil
}

func (s *Service) runBlueToGreenAll(jobID, actor, reason string) {
	ctx := context.Background()
	out, err := s.ssh.SyncBlueToGreenAllComponents(ctx)
	if err != nil {
		s.finishJob(jobID, StatusFailed, err.Error(), out)
		_ = s.store.AddAudit(ctx, actor, "component_sync_blue_to_green_failed", jobID, err.Error())
		return
	}
	s.finishJob(jobID, StatusSuccess, "蓝→绿所有组件增量同步完成", out)
	_ = s.store.AddAudit(ctx, actor, "component_sync_blue_to_green_done", jobID, out)
	if reason != "" {
		_ = s.store.AddAudit(ctx, actor, "component_sync_blue_to_green_reason", jobID, reason)
	}
}

func (s *Service) finishJob(id string, status JobStatus, message, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil || s.active.ID != id {
		return
	}
	s.active.Status = status
	s.active.Message = message
	s.active.Output = output
	s.active.EndedAt = time.Now().UTC()
}

func (s *Service) snapshotJob(id string) *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.active == nil || s.active.ID != id {
		return nil
	}
	j := *s.active
	return &j
}
