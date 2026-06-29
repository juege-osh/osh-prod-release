package release

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/juege/osh-prod-release/internal/approval"
	"github.com/juege/osh-prod-release/internal/component"
	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/github"
	"github.com/juege/osh-prod-release/internal/models"
	"github.com/juege/osh-prod-release/internal/ssh"
	"github.com/juege/osh-prod-release/internal/store"
	"github.com/juege/osh-prod-release/internal/testrunner"
	"github.com/juege/osh-prod-release/internal/traffic"
)

type Service struct {
	cfg       *config.Config
	store     *store.Store
	approval  *approval.Engine
	ssh       *ssh.Client
	test      *testrunner.Runner
	component *component.Service
	artifact  *github.ArtifactService
	deployGH  *github.DeployTrigger
	traffic   *traffic.Service
	mu        sync.Mutex
	jobs      deployJobRegistry
}

type componentAppliedItem struct {
	item models.ChangeItem
	ex   component.Executor
}

func New(cfg *config.Config, st *store.Store, trafficSvc *traffic.Service) *Service {
	return &Service{
		cfg:       cfg,
		store:     st,
		approval:  approval.New(cfg.BossReviewer),
		ssh:       ssh.New(cfg),
		test:      testrunner.New(cfg),
		component: component.NewService(cfg, ssh.New(cfg)),
		artifact:  github.New(cfg),
		deployGH:  github.NewDeployTrigger(cfg),
		traffic:   trafficSvc,
	}
}

func (s *Service) Store() *store.Store { return s.store }

func (s *Service) Create(ctx context.Context, req models.CreateReleaseRequest) (*models.Release, error) {
	if req.Title == "" || req.CommitSHA == "" || req.Author == "" {
		return nil, fmt.Errorf("title, commit_sha, author required")
	}
	if req.Level == "" {
		req.Level = models.LevelNormal
	}
	if _, err := s.artifact.Resolve(ctx, req.Repo, req.CommitSHA); err != nil {
		return nil, err
	}
	rel, err := s.store.CreateRelease(ctx, req)
	if err != nil {
		return nil, err
	}
	_ = s.store.AddAudit(ctx, req.Author, "create_release", rel.ID, rel.Title)
	return rel, nil
}

func (s *Service) ApplyAdminFastTrack(ctx context.Context, id, actor string) (*models.Release, error) {
	if err := s.store.SetBossApproved(ctx, id, actor); err != nil {
		return nil, err
	}
	_ = s.store.UpdateStep(ctx, id, "submit_review", "skipped", "管理员直通发布")
	_ = s.store.UpdateStep(ctx, id, "item_reviews", "skipped", "管理员直通发布")
	_ = s.store.UpdateStep(ctx, id, "boss_approve", "skipped", "管理员直通发布")
	if err := s.store.UpdateReleaseStatus(ctx, id, models.StatusApproved); err != nil {
		return nil, err
	}
	_ = s.store.AddAudit(ctx, actor, "admin_fast_track", id, "跳过双评审与终审")
	return s.store.GetRelease(ctx, id)
}

func (s *Service) Get(ctx context.Context, id string) (*models.Release, error) {
	return s.store.GetRelease(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]models.Release, error) {
	return s.store.ListReleases(ctx)
}

func (s *Service) SubmitForReview(ctx context.Context, id, actor string) (*models.Release, error) {
	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return nil, err
	}
	if rel.Status != models.StatusDraft {
		return nil, fmt.Errorf("only draft can submit review")
	}
	if err := s.store.UpdateReleaseStatus(ctx, id, models.StatusReviewing); err != nil {
		return nil, err
	}
	_ = s.store.UpdateStep(ctx, id, "submit_review", "success", "已提交评审")
	_ = s.store.AddAudit(ctx, actor, "submit_review", id, "")
	return s.store.GetRelease(ctx, id)
}

func (s *Service) SubmitReview(ctx context.Context, itemID string, req models.SubmitReviewRequest) (*models.Release, error) {
	item, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	rel, err := s.store.GetRelease(ctx, item.ReleaseID)
	if err != nil {
		return nil, err
	}
	if rel.Status != models.StatusReviewing && rel.Status != models.StatusDraft {
		return nil, fmt.Errorf("release not in reviewing state")
	}
	if req.Reviewer != item.Reviewer1 && req.Reviewer != item.Reviewer2 {
		return nil, fmt.Errorf("reviewer not assigned to this item")
	}
	if req.Result == models.ReviewApprove && !req.Tested {
		return nil, fmt.Errorf("must confirm tested before approve")
	}
	if item.DemoRequired && req.Reviewer != item.Developer && req.Result == models.ReviewApprove && !req.DemoSeen {
		return nil, fmt.Errorf("must confirm demo seen")
	}
	if _, err := s.store.AddReview(ctx, itemID, req); err != nil {
		return nil, err
	}
	_ = s.store.AddAudit(ctx, req.Reviewer, "submit_item_review", itemID, string(req.Result))

	rel, _ = s.store.GetRelease(ctx, item.ReleaseID)
	ok, msg := s.approval.AllItemsReviewOK(rel.Items)
	if ok {
		_ = s.store.UpdateStep(ctx, rel.ID, "item_reviews", "success", "双评审通过")
		_ = s.store.MarkItemsApproved(ctx, rel.ID)
	} else {
		_ = s.store.UpdateStep(ctx, rel.ID, "item_reviews", "running", msg)
	}
	return s.store.GetRelease(ctx, item.ReleaseID)
}

func (s *Service) BossApprove(ctx context.Context, id string, req models.BossApproveRequest, actorIsBoss bool) (*models.Release, error) {
	if !actorIsBoss {
		boss := s.cfg.BossReviewer
		if boss == "" {
			boss = "juege"
		}
		return nil, fmt.Errorf("仅 %s 可终审通过", boss)
	}
	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Reviewer == "" {
		req.Reviewer = s.cfg.BossReviewer
	}
	ok, msg := s.approval.AllItemsReviewOK(rel.Items)
	if !ok {
		return nil, fmt.Errorf(msg)
	}
	if s.approval.NeedsPerItemBossApproval(rel.Level) {
		if ok, msg := s.approval.AllItemsBossApprovalOK(rel.Items); !ok {
			return nil, fmt.Errorf(msg)
		}
	}
	if err := s.store.SetBossApproved(ctx, id, req.Reviewer); err != nil {
		return nil, err
	}
	_ = s.store.UpdateStep(ctx, id, "boss_approve", "success", req.Comment)
	_ = s.store.AddAudit(ctx, req.Reviewer, "boss_approve", id, req.Comment)
	return s.store.GetRelease(ctx, id)
}

func (s *Service) BossApproveItem(ctx context.Context, itemID string, req models.ItemBossApproveRequest, actorIsBoss bool) (*models.Release, error) {
	if !actorIsBoss {
		boss := s.cfg.BossReviewer
		if boss == "" {
			boss = "juege"
		}
		return nil, fmt.Errorf("仅 %s 可逐项确认紧急上线项", boss)
	}
	item, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	rel, err := s.store.GetRelease(ctx, item.ReleaseID)
	if err != nil {
		return nil, err
	}
	if !s.approval.NeedsPerItemBossApproval(rel.Level) {
		return nil, fmt.Errorf("常规上线不需要逐项觉哥确认")
	}
	if req.Reviewer == "" {
		req.Reviewer = s.cfg.BossReviewer
	}
	if err := s.store.SetItemBossApproved(ctx, itemID, req.Reviewer); err != nil {
		return nil, err
	}
	_ = s.store.AddAudit(ctx, req.Reviewer, "boss_approve_item", itemID, req.Comment)
	return s.store.GetRelease(ctx, item.ReleaseID)
}

func (s *Service) GetActiveDeploy(ctx context.Context) (*models.Release, error) {
	return s.store.GetActiveDeployingRelease(ctx)
}

// StartDeploy kicks off deploy in background and returns immediately (frontend polls status).
func (s *Service) StartDeploy(ctx context.Context, id, actor string, adminBypass bool) (*models.Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	active, err := s.store.GetActiveDeployingRelease(ctx)
	if err != nil {
		return nil, err
	}
	if active != nil {
		if active.ID != id {
			return nil, fmt.Errorf("已有发布单「%s」正在部署中，请等待完成后再部署", active.Title)
		}
		return nil, fmt.Errorf("当前发布单正在部署中，请等待完成")
	}

	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return nil, err
	}
	ok, msg := s.approval.CanStartDeploy(*rel, adminBypass)
	if !ok {
		return nil, fmt.Errorf(msg)
	}

	target := rel.DeployTarget
	if target == "" {
		target = "green"
	}
	if target == "blue" {
		if s.traffic == nil {
			return nil, fmt.Errorf("traffic service not configured")
		}
		if err := s.traffic.RequireProductionGreen(ctx); err != nil {
			return nil, err
		}
	}

	deployMsg := "审批已通过，准备部署到绿环境"
	if target == "blue" {
		deployMsg = "审批已通过，准备部署到蓝环境"
	}

	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusDeploying)
	_ = s.store.UpdateStep(ctx, id, "deploy_standby", "running", deployMsg)

	deployCtx, cancel := context.WithCancel(context.Background())
	job := &deployJob{
		releaseID: id,
		target:    target,
		cancel:    cancel,
	}
	s.jobs.set(job)

	go s.executeDeploy(deployCtx, id, actor)
	return s.store.GetRelease(ctx, id)
}

func (s *Service) CancelDeploy(ctx context.Context, id, actor string) (*models.Release, error) {
	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return nil, err
	}
	if rel.Status != models.StatusDeploying && rel.Status != models.StatusTesting {
		return nil, fmt.Errorf("当前发布单不在部署中，无法终止")
	}

	job, ok := s.jobs.stop(id)
	dispatchSince := time.Now().UTC().Add(-30 * time.Minute)
	target := rel.DeployTarget
	if target == "" {
		target = "green"
	}
	if ok && job != nil {
		if !job.dispatchSince.IsZero() {
			dispatchSince = job.dispatchSince
		}
		if job.target != "" {
			target = job.target
		}
	}

	_ = s.store.UpdateStep(ctx, id, "deploy_standby", "running", "正在终止部署并回滚到部署前版本…")
	if rel.Status == models.StatusTesting {
		_ = s.store.UpdateStep(ctx, id, "auto_test", "pending", "已终止")
	}

	go s.finishCancelDeploy(id, actor, target, dispatchSince)
	return s.store.GetRelease(ctx, id)
}

func (s *Service) finishCancelDeploy(id, actor, target string, dispatchSince time.Time) {
	ctx := context.Background()
	var detail []string

	if out, err := s.deployGH.CancelActiveWorkflows(ctx, dispatchSince); err != nil {
		detail = append(detail, "cancel GHA: "+err.Error())
	} else {
		detail = append(detail, out)
	}

	revertOut, revertErr := s.revertToLatestSnapshot(ctx, target, id, actor)
	if revertErr != nil {
		detail = append(detail, "revert: "+revertErr.Error())
	} else if revertOut != "" {
		detail = append(detail, revertOut)
	}

	msg := "部署已终止，已恢复部署前版本，可重新点击部署"
	if revertErr != nil {
		msg = "部署已终止（自动回滚失败: " + revertErr.Error() + "，请手动检查环境），可重新点击部署"
	}

	_ = s.store.UpdateStep(ctx, id, "deploy_standby", "pending", msg)
	_ = s.store.UpdateStep(ctx, id, "auto_test", "pending", "")
	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusApproved)
	_ = s.store.AddAudit(ctx, actor, "deploy_cancel", id, strings.Join(detail, "; "))
	s.jobs.clear(id)
	s.jobs.clearCancelled(id)
}

func (s *Service) revertToLatestSnapshot(ctx context.Context, target, releaseID, actor string) (string, error) {
	snap, err := s.store.GetLatestDeploySnapshot(ctx, target)
	if err != nil {
		return "", err
	}

	backendRef := snap.BackendGitRef
	frontendRef := snap.FrontendGitRef
	if backendRef == "" {
		backendRef = s.cfg.GitHubBackendGitRef
	}
	if frontendRef == "" {
		frontendRef = s.cfg.GitHubFrontendGitRef
	}

	dispatchSince := time.Now().UTC().Add(-15 * time.Second)
	out, errDeploy := s.deployGH.TriggerSlot149(ctx, backendRef, frontendRef, "revert-"+releaseID, target)
	if errDeploy != nil {
		return "", errDeploy
	}
	ghaOut, waitErr := s.deployGH.WaitSlotWorkflows(ctx, dispatchSince, 30*time.Minute)
	if waitErr != nil {
		return "", waitErr
	}
	port := "28080"
	if target == "blue" {
		port = "58080"
	}
	waitOut, waitErr := s.ssh.WaitSlotAPI(ctx, target, port, 5*time.Minute)
	if waitErr != nil {
		return "", waitErr
	}
	return out + "; GHA: " + ghaOut + "; " + waitOut, nil
}

func (s *Service) deployWasCancelled(id string) bool {
	return s.jobs.isCancelled(id)
}

func (s *Service) executeDeploy(ctx context.Context, id, actor string) {
	defer s.jobs.clear(id)

	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return
	}
	target := rel.DeployTarget
	if target == "" {
		target = "green"
	}
	if target == "blue" && s.traffic != nil {
		if err := s.traffic.RequireProductionGreen(ctx); err != nil {
			if s.deployWasCancelled(id) {
				return
			}
			_ = s.store.UpdateStep(ctx, id, "deploy_standby", "failed", err.Error())
			_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusFailed)
			return
		}
	}

	var out string
	var errDeploy error
	if target == "blue" {
		out, errDeploy = s.executeBlueDeploy(ctx, id)
	} else {
		out, errDeploy = s.executeGreenDeploy(ctx, id)
	}
	if s.deployWasCancelled(id) || errors.Is(errDeploy, context.Canceled) {
		return
	}
	if errDeploy != nil {
		_ = s.store.UpdateStep(ctx, id, "deploy_standby", "failed", errDeploy.Error())
		_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusFailed)
		return
	}
	if target == "green" {
		if compOut, compErr := s.executeComponentChanges(ctx, *rel, "green", actor); compErr != nil {
			_ = s.store.UpdateStep(ctx, id, "deploy_standby", "failed", compErr.Error())
			_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusFailed)
			return
		} else if compOut != "" {
			out = out + "; components: " + compOut
		}
	}
	_ = s.store.UpdateStep(ctx, id, "deploy_standby", "success", out)
	_ = s.store.AddAudit(ctx, actor, "deploy_standby", id, out)

	if s.deployWasCancelled(id) {
		return
	}

	env := "green"
	if target == "blue" {
		env = "blue"
	}
	_, _ = s.runAutoTest(ctx, id, actor, env, target)
}

func (s *Service) executeGreenDeploy(ctx context.Context, id string) (string, error) {
	var out string
	var errDeploy error
	if s.cfg.GitHubToken != "" && (s.cfg.GitHubBackendRepo != "" || s.cfg.GitHubFrontendRepo != "") {
		dispatchSince := time.Now().UTC().Add(-15 * time.Second)
		s.jobs.markDispatchSince(id, dispatchSince)
		out, errDeploy = s.deployGH.TriggerGreen149(ctx, "", "", id)
		if errDeploy == nil {
			_ = s.store.UpdateStep(ctx, id, "deploy_standby", "running", "GHA 已触发，等待前后端 workflow 跑完（约 3–8 分钟）…")
			ghaOut, waitErr := s.deployGH.WaitGreenWorkflows(ctx, dispatchSince, 25*time.Minute)
			if waitErr != nil {
				errDeploy = waitErr
			} else {
				out = out + "; GHA: " + ghaOut
				_ = s.store.UpdateStep(ctx, id, "deploy_standby", "running", "GHA 已完成，149 上 patch 端口/Nacos 并重启…")
				postOut, postErr := s.ssh.SlotPostDeploy(ctx, "green")
				if postErr != nil {
					errDeploy = postErr
				} else {
					out = out + "; postdeploy: " + postOut
					_ = s.store.UpdateStep(ctx, id, "deploy_standby", "running", "postdeploy 完成，验证绿环境 HTTP…")
					waitOut, waitErr := s.ssh.WaitGreenAPI(ctx, "28080", 3*time.Minute)
					if waitErr != nil {
						errDeploy = waitErr
					} else {
						out = out + "; " + waitOut
					}
				}
			}
		}
	} else {
		out, errDeploy = s.ssh.DeployGreenCode(ctx)
	}
	return out, errDeploy
}

func (s *Service) executeBlueDeploy(ctx context.Context, id string) (string, error) {
	var out string
	var errDeploy error
	if s.cfg.GitHubToken != "" && (s.cfg.GitHubBackendRepo != "" || s.cfg.GitHubFrontendRepo != "") {
		dispatchSince := time.Now().UTC().Add(-15 * time.Second)
		s.jobs.markDispatchSince(id, dispatchSince)
		out, errDeploy = s.deployGH.TriggerSlot149(ctx, "", "", id, "blue")
		if errDeploy == nil {
			_ = s.store.UpdateStep(ctx, id, "deploy_standby", "running", "GHA 已触发，等待前后端 workflow 跑完（约 3–8 分钟）…")
			ghaOut, waitErr := s.deployGH.WaitSlotWorkflows(ctx, dispatchSince, 30*time.Minute)
			if waitErr != nil {
				errDeploy = waitErr
			} else {
				out = out + "; GHA: " + ghaOut
				_ = s.store.UpdateStep(ctx, id, "deploy_standby", "running", "GHA 已完成，149 上 patch 端口/Nacos 并重启…")
				postOut, postErr := s.ssh.SlotPostDeploy(ctx, "blue")
				if postErr != nil {
					errDeploy = postErr
				} else {
					out = out + "; postdeploy: " + postOut
					_ = s.store.UpdateStep(ctx, id, "deploy_standby", "running", "postdeploy 完成，验证蓝环境 :58080…")
					waitOut, waitErr := s.ssh.WaitSlotAPI(ctx, "blue", "58080", 5*time.Minute)
					if waitErr != nil {
						errDeploy = waitErr
					} else {
						out = out + "; " + waitOut
					}
				}
			}
		}
	} else {
		out, errDeploy = s.ssh.DeployBlueCode(ctx)
	}
	return out, errDeploy
}

func (s *Service) executeComponentChanges(ctx context.Context, rel models.Release, slot, actor string) (string, error) {
	if s.component == nil {
		return "", nil
	}
	var parts []string
	var applied []componentAppliedItem
	for _, item := range component.SortItems(rel.Items) {
		ex := s.component.ExecutorFor(item)
		if ex == nil {
			continue
		}
		plan, err := ex.Plan(ctx, rel, item)
		if err != nil {
			return strings.Join(parts, "; "), err
		}
		exec, err := s.store.UpsertChangeExecution(ctx, models.ChangeExecution{
			ReleaseID: rel.ID,
			ItemID:    item.ID,
			Slot:      slot,
			Component: plan.Component,
			Action:    plan.Action,
			Node:      plan.Node,
			Status:    "planned",
			PlanJSON:  component.MarshalResult(plan),
			StartedAt: time.Now().UTC(),
		})
		if err != nil {
			return strings.Join(parts, "; "), err
		}

		snap, err := ex.Snapshot(ctx, rel, item, slot)
		if err != nil {
			s.finishComponentExecution(ctx, exec, "failed", snap.Output, err.Error())
			return strings.Join(parts, "; "), err
		}
		_ = s.store.SaveRollbackSnapshot(ctx, models.RollbackSnapshot{
			ReleaseID:    rel.ID,
			ItemID:       item.ID,
			Slot:         slot,
			Component:    snap.Component,
			SnapshotType: string(snap.Phase),
			MetadataJSON: component.MarshalResult(snap),
		})

		var apply component.Result
		if slot == "blue" {
			apply, err = ex.ApplyBlue(ctx, rel, item)
		} else {
			apply, err = ex.ApplyGreen(ctx, rel, item)
		}
		if err != nil {
			s.finishComponentExecution(ctx, exec, "failed", apply.Output, err.Error())
			s.rollbackAppliedComponents(ctx, rel, slot, actor, applied)
			return strings.Join(parts, "; "), err
		}
		applied = append(applied, componentAppliedItem{item: item, ex: ex})

		test, err := ex.Test(ctx, rel, item, slot)
		if err != nil {
			s.finishComponentExecution(ctx, exec, "failed", test.Output, err.Error())
			s.rollbackAppliedComponents(ctx, rel, slot, actor, applied)
			return strings.Join(parts, "; "), err
		}
		_ = s.store.SaveComponentTestReport(ctx, models.ComponentTestReport{
			ReleaseID:      rel.ID,
			ItemID:         item.ID,
			Slot:           slot,
			Component:      test.Component,
			FunctionalJSON: component.MarshalResult(test),
			DataDiffJSON:   component.MarshalResult(apply),
			AIVerdict:      componentVerdict(item, test, apply),
			Passed:         test.Status != "failed" && apply.Status != "failed",
		})
		finished := time.Now().UTC()
		exec.Status = "success"
		exec.Output = apply.Output
		exec.FinishedAt = &finished
		_, _ = s.store.UpsertChangeExecution(ctx, exec)
		_ = s.store.UpdateItemStatus(ctx, item.ID, models.ItemStatusDeployed)
		parts = append(parts, fmt.Sprintf("%s/%s:%s", slot, test.Component, test.Status))
		_ = s.store.AddAudit(ctx, actor, "component_change_"+slot, item.ID, component.MarshalResult(apply))
	}
	return strings.Join(parts, "; "), nil
}

func (s *Service) rollbackAppliedComponents(ctx context.Context, rel models.Release, slot, actor string, applied []componentAppliedItem) {
	for i := len(applied) - 1; i >= 0; i-- {
		entry := applied[i]
		var res component.Result
		var err error
		if slot == "blue" {
			res, err = entry.ex.RollbackBlue(ctx, rel, entry.item)
		} else {
			res, err = entry.ex.RollbackGreen(ctx, rel, entry.item)
		}
		detail := component.MarshalResult(res)
		if err != nil {
			detail = detail + "; rollback_error=" + err.Error()
		}
		_ = s.store.AddAudit(ctx, actor, "component_rollback_"+slot, entry.item.ID, detail)
	}
}

func (s *Service) RollbackItem(ctx context.Context, itemID, slot, actor string) (component.Result, error) {
	if slot == "" {
		slot = "green"
	}
	if slot != "green" && slot != "blue" {
		return component.Result{}, fmt.Errorf("rollback slot must be green or blue")
	}
	item, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return component.Result{}, err
	}
	rel, err := s.store.GetRelease(ctx, item.ReleaseID)
	if err != nil {
		return component.Result{}, err
	}
	ex := s.component.ExecutorFor(*item)
	if ex == nil {
		return component.Result{}, fmt.Errorf("上线项 %s 没有可回滚的组件执行器", item.Title)
	}
	started := time.Now().UTC()
	exec, err := s.store.UpsertChangeExecution(ctx, models.ChangeExecution{
		ReleaseID: rel.ID,
		ItemID:    item.ID,
		Slot:      slot,
		Component: item.Component,
		Action:    "rollback",
		Node:      item.TargetNode,
		Status:    "running",
		StartedAt: started,
	})
	if err != nil {
		return component.Result{}, err
	}
	var res component.Result
	if slot == "blue" {
		res, err = ex.RollbackBlue(ctx, *rel, *item)
	} else {
		res, err = ex.RollbackGreen(ctx, *rel, *item)
	}
	status := "success"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}
	s.finishComponentExecution(ctx, exec, status, res.Output, errMsg)
	_ = s.store.SaveComponentTestReport(ctx, models.ComponentTestReport{
		ReleaseID:      rel.ID,
		ItemID:         item.ID,
		Slot:           slot,
		Component:      res.Component,
		FunctionalJSON: component.MarshalResult(res),
		DataDiffJSON:   component.MarshalResult(res),
		AIVerdict:      componentVerdict(*item, res, res),
		Passed:         err == nil && res.Status != "failed",
	})
	_ = s.store.AddAudit(ctx, actor, "component_manual_rollback_"+slot, item.ID, component.MarshalResult(res))
	return res, err
}

func (s *Service) finishComponentExecution(ctx context.Context, e models.ChangeExecution, status, output, errMsg string) {
	finished := time.Now().UTC()
	e.Status = status
	e.Output = output
	e.Error = errMsg
	e.FinishedAt = &finished
	_, _ = s.store.UpsertChangeExecution(ctx, e)
}

func componentVerdict(item models.ChangeItem, test, apply component.Result) string {
	expected := strings.TrimSpace(item.DataImpact)
	if expected == "" {
		expected = strings.TrimSpace(item.ExpectedImpact)
	}
	if test.Status == "failed" || apply.Status == "failed" {
		return "AI 判定：组件执行或测试失败，需要负责人复核。预期影响：" + expected
	}
	if expected == "" {
		return "AI 判定：组件执行成功；未填写数据影响，建议负责人补充预期影响。"
	}
	return "AI 判定：组件执行成功；实际结果需按报告核对是否符合预期影响：" + expected
}

func (s *Service) runAutoTest(ctx context.Context, id, actor, env, target string) (*models.Release, error) {
	if s.deployWasCancelled(id) {
		return nil, context.Canceled
	}
	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusTesting)
	_ = s.store.UpdateStep(ctx, id, "auto_test", "running", "")

	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return nil, err
	}
	report, err := s.test.Run(ctx, *rel, env)
	if s.deployWasCancelled(id) {
		return nil, context.Canceled
	}
	if err != nil {
		_ = s.store.UpdateStep(ctx, id, "auto_test", "failed", err.Error())
		_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusFailed)
		return nil, err
	}
	_ = s.store.SaveTestReport(ctx, *report)
	if s.deployWasCancelled(id) {
		return nil, context.Canceled
	}
	if !report.Passed {
		_ = s.store.UpdateStep(ctx, id, "auto_test", "failed", report.AIVerdict)
		_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusFailed)
		return s.store.GetRelease(ctx, id)
	}
	_ = s.store.UpdateStep(ctx, id, "auto_test", "success", report.AIVerdict)
	_ = s.store.AddAudit(ctx, actor, "auto_test_pass", id, report.AIVerdict)
	if s.deployWasCancelled(id) {
		return nil, context.Canceled
	}
	if target == "blue" {
		return s.finishBlueDeploy(ctx, id, actor, report.AIVerdict)
	}
	return s.finishGreenDeploy(ctx, id, actor, report.AIVerdict)
}

// finishGreenDeploy marks a green-only pre-release deploy as done (no prod traffic switch).
func (s *Service) finishGreenDeploy(ctx context.Context, id, actor, msg string) (*models.Release, error) {
	_ = s.store.UpdateStep(ctx, id, "switch_traffic", "pending", "绿环境已通过自动测试，等待一键切流")
	_ = s.store.UpdateStep(ctx, id, "manual_verify", "pending", "切流后由负责人生产人工复测")
	_ = s.store.UpdateStep(ctx, id, "sync_standby", "pending", "人工复测通过后同步同一批 change 到蓝环境")
	_ = s.store.UpdateStep(ctx, id, "finish", "pending", "")
	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusTesting)
	_ = s.store.AddAudit(ctx, actor, "green_ready_to_switch", id, msg)
	s.recordDeploySnapshot(ctx, id, actor, "green")
	return s.store.GetRelease(ctx, id)
}

// finishBlueDeploy marks a blue standby deploy as done (no prod traffic switch).
func (s *Service) finishBlueDeploy(ctx context.Context, id, actor, msg string) (*models.Release, error) {
	_ = s.store.UpdateStep(ctx, id, "switch_traffic", "skipped", "蓝环境待命，不切生产流量")
	_ = s.store.UpdateStep(ctx, id, "manual_verify", "skipped", "请验收 :58080")
	_ = s.store.UpdateStep(ctx, id, "sync_standby", "skipped", "数据同步请使用「同步数据库」")
	_ = s.store.UpdateStep(ctx, id, "finish", "success", "蓝环境部署完成")
	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusDone)
	_ = s.store.AddAudit(ctx, actor, "blue_deploy_done", id, msg)
	s.recordDeploySnapshot(ctx, id, actor, "blue")
	return s.store.GetRelease(ctx, id)
}

func (s *Service) SwitchTraffic(ctx context.Context, id, actor string) (*models.Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return nil, err
	}
	if rel.Status != models.StatusTesting {
		return nil, fmt.Errorf("auto tests must pass before switch (status=%s)", rel.Status)
	}

	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusSwitching)
	_ = s.store.UpdateStep(ctx, id, "switch_traffic", "running", "")

	out, err := s.ssh.SwitchToGreen(ctx)
	if err != nil {
		_ = s.store.UpdateStep(ctx, id, "switch_traffic", "failed", err.Error())
		_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusFailed)
		return nil, err
	}
	_ = s.store.SetActiveSlot(ctx, id, "green")
	_ = s.store.UpdateStep(ctx, id, "switch_traffic", "success", out)
	_ = s.store.AddSwitchEvent(ctx, models.SwitchEvent{
		ID: uuid.New().String()[:12], ReleaseID: id,
		FromSlot: "blue", ToSlot: "green", Actor: actor, Reason: "deploy switch",
		CreatedAt: time.Now().UTC(),
	})
	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusVerifying)
	_ = s.store.UpdateStep(ctx, id, "manual_verify", "running", "等待功能负责人生产人工复测")
	_ = s.store.AddAudit(ctx, actor, "switch_traffic", id, out)
	return s.store.GetRelease(ctx, id)
}

func (s *Service) ConfirmManualVerify(ctx context.Context, id, actor string) (*models.Release, error) {
	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return nil, err
	}
	if rel.Status != models.StatusVerifying {
		return nil, fmt.Errorf("not in verifying state")
	}
	_ = s.store.UpdateStep(ctx, id, "manual_verify", "success", "人工复测通过")
	return s.SyncToStandby(ctx, id, actor)
}

func (s *Service) SyncToStandby(ctx context.Context, id, actor string) (*models.Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.store.GetRelease(ctx, id); err != nil {
		return nil, err
	}
	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusSyncing)
	_ = s.store.UpdateStep(ctx, id, "sync_standby", "running", "同一 commit 同步到蓝槽位")

	var out string
	var errSync error
	if s.cfg.GitHubToken != "" && (s.cfg.GitHubBackendRepo != "" || s.cfg.GitHubFrontendRepo != "") {
		out, errSync = s.deployGH.TriggerSlot149(ctx, "", "", id, "blue")
		if errSync == nil {
			waitOut, waitErr := s.ssh.WaitSlotAPI(ctx, "blue", "58080", 25*time.Minute)
			if waitErr != nil {
				errSync = waitErr
			} else {
				out = out + "; " + waitOut
			}
		}
	} else {
		out, errSync = s.ssh.DeployBlueCode(ctx)
	}
	if errSync == nil {
		rel, _ := s.store.GetRelease(ctx, id)
		if rel != nil {
			compOut, compErr := s.executeComponentChanges(ctx, *rel, "blue", actor)
			if compErr != nil {
				errSync = compErr
			} else if compOut != "" {
				out = out + "; components: " + compOut
			}
		}
	}
	if errSync != nil {
		_ = s.store.UpdateStep(ctx, id, "sync_standby", "failed", errSync.Error())
		_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusFailed)
		return nil, errSync
	}
	_ = s.store.UpdateStep(ctx, id, "sync_standby", "success", out)
	_ = s.store.UpdateStep(ctx, id, "finish", "success", "发布完成")
	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusDone)
	_ = s.store.AddAudit(ctx, actor, "release_done", id, out)
	return s.store.GetRelease(ctx, id)
}

func (s *Service) Rollback(ctx context.Context, id string, req models.ActionRequest) (*models.Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		return nil, err
	}
	out, err := s.ssh.SwitchToBlue(ctx)
	if err != nil {
		return nil, err
	}
	_ = s.store.SetActiveSlot(ctx, id, "blue")
	_ = s.store.AddSwitchEvent(ctx, models.SwitchEvent{
		ID: uuid.New().String()[:12], ReleaseID: id,
		FromSlot: rel.ActiveSlot, ToSlot: "blue", Actor: req.Actor, Reason: req.Reason,
		CreatedAt: time.Now().UTC(),
	})
	_ = s.store.UpdateReleaseStatus(ctx, id, models.StatusRolledBack)
	_ = s.store.AddAudit(ctx, req.Actor, "rollback", id, out+" "+req.Reason)
	return s.store.GetRelease(ctx, id)
}

func (s *Service) TrafficStatus(ctx context.Context) (string, error) {
	return s.ssh.TrafficStatus(ctx)
}

func (s *Service) ListDeploySnapshots(ctx context.Context, target string) ([]models.DeploySnapshot, error) {
	return s.store.ListDeploySnapshots(ctx, target, 20)
}

func (s *Service) recordDeploySnapshot(ctx context.Context, releaseID, actor, target string) {
	rel, err := s.store.GetRelease(ctx, releaseID)
	if err != nil {
		return
	}
	title := rel.Title
	if title == "" {
		title = releaseID
	}
	_, _ = s.store.SaveDeploySnapshot(ctx, models.DeploySnapshot{
		ReleaseID:      releaseID,
		DeployTarget:   target,
		Title:          title,
		BackendGitRef:  s.cfg.GitHubBackendGitRef,
		FrontendGitRef: s.cfg.GitHubFrontendGitRef,
		Actor:          actor,
		Status:         "success",
	})
}

func (s *Service) RollbackDeploy(ctx context.Context, req models.DeployRollbackRequest, adminBypass bool) (*models.DeploySnapshot, error) {
	if !adminBypass {
		return nil, fmt.Errorf("仅管理员可执行部署版本回滚")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	target := strings.ToLower(strings.TrimSpace(req.Target))
	if target == "" {
		target = "green"
	}
	if target != "green" && target != "blue" {
		return nil, fmt.Errorf("target must be green or blue")
	}
	if target == "blue" && s.traffic != nil {
		if err := s.traffic.RequireProductionGreen(ctx); err != nil {
			return nil, err
		}
	}

	var snap *models.DeploySnapshot
	var err error
	if req.SnapshotID != "" {
		snap, err = s.store.GetDeploySnapshot(ctx, req.SnapshotID)
	} else {
		snap, err = s.store.GetPreviousDeploySnapshot(ctx, target)
	}
	if err != nil {
		return nil, err
	}
	if snap.DeployTarget != target {
		return nil, fmt.Errorf("快照目标环境不匹配")
	}

	backendRef := snap.BackendGitRef
	frontendRef := snap.FrontendGitRef
	if backendRef == "" {
		backendRef = s.cfg.GitHubBackendGitRef
	}
	if frontendRef == "" {
		frontendRef = s.cfg.GitHubFrontendGitRef
	}

	dispatchSince := time.Now().UTC().Add(-15 * time.Second)
	out, errDeploy := s.deployGH.TriggerSlot149(ctx, backendRef, frontendRef, "rollback-"+snap.ID, target)
	if errDeploy != nil {
		return nil, errDeploy
	}
	ghaOut, waitErr := s.deployGH.WaitSlotWorkflows(ctx, dispatchSince, 30*time.Minute)
	if waitErr != nil {
		return nil, waitErr
	}
	port := "28080"
	if target == "blue" {
		port = "58080"
	}
	waitOut, waitErr := s.ssh.WaitSlotAPI(ctx, target, port, 5*time.Minute)
	if waitErr != nil {
		return nil, waitErr
	}

	actor := req.Actor
	if actor == "" {
		actor = "ops"
	}
	rolled, err := s.store.SaveDeploySnapshot(ctx, models.DeploySnapshot{
		ReleaseID:      snap.ReleaseID,
		DeployTarget:   target,
		Title:          "回滚→" + snap.Title,
		BackendGitRef:  backendRef,
		FrontendGitRef: frontendRef,
		BackendSHA:     snap.BackendSHA,
		FrontendSHA:    snap.FrontendSHA,
		Actor:          actor,
		Status:         "success",
	})
	if err != nil {
		return nil, err
	}
	detail := out + "; GHA: " + ghaOut + "; " + waitOut
	if req.Reason != "" {
		detail += "; reason: " + req.Reason
	}
	_ = s.store.AddAudit(ctx, actor, "deploy_rollback", rolled.ID, detail)
	return rolled, nil
}
