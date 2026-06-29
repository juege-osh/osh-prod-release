package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/juege/osh-prod-release/internal/auth"
	"github.com/juege/osh-prod-release/internal/blue"
	"github.com/juege/osh-prod-release/internal/componentops"
	"github.com/juege/osh-prod-release/internal/componentsync"
	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/github"
	"github.com/juege/osh-prod-release/internal/migrate"
	"github.com/juege/osh-prod-release/internal/models"
	"github.com/juege/osh-prod-release/internal/notify"
	"github.com/juege/osh-prod-release/internal/release"
	"github.com/juege/osh-prod-release/internal/ssh"
	"github.com/juege/osh-prod-release/internal/traffic"
)

type Handler struct {
	cfg     *config.Config
	release *release.Service
	traffic *traffic.Service
	blue    *blue.Service
	sync    *componentsync.Service
	migrate *migrate.Runner
	authSvc *auth.Service
	notify  *notify.Service
	compOps *componentops.Service
}

func New(cfg *config.Config, svc *release.Service, authSvc *auth.Service) *Handler {
	sshClient := ssh.New(cfg)
	trafficSvc := traffic.New(svc.Store(), sshClient)
	return &Handler{
		cfg:     cfg,
		release: svc,
		traffic: trafficSvc,
		blue:    blue.New(cfg, svc.Store(), sshClient, github.NewDeployTrigger(cfg), trafficSvc),
		sync:    componentsync.New(svc.Store(), sshClient, trafficSvc),
		migrate: migrate.NewRunner(cfg, sshClient),
		authSvc: authSvc,
		notify:  notify.New(cfg),
		compOps: componentops.New(cfg, svc.Store(), sshClient),
	}
}

func (h *Handler) bearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

func (h *Handler) currentUser(r *http.Request) (*models.User, bool) {
	if h.authSvc == nil {
		return nil, false
	}
	token := h.bearerToken(r)
	if token == "" {
		return nil, false
	}
	u, err := h.authSvc.UserFromToken(r.Context(), token)
	if err != nil {
		return nil, false
	}
	u.IsAdmin = h.authSvc.IsAdmin(u)
	u.IsBoss = h.authSvc.IsBoss(u)
	return u, true
}

func (h *Handler) auth(w http.ResponseWriter, r *http.Request) bool {
	if u, ok := h.currentUser(r); ok {
		return u != nil
	}
	if h.cfg.APIToken == "" {
		return true
	}
	if h.bearerToken(r) == h.cfg.APIToken {
		return true
	}
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	return false
}

func (h *Handler) requireUser(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	if u, ok := h.currentUser(r); ok {
		return u, true
	}
	if h.cfg.APIToken != "" && h.bearerToken(r) == h.cfg.APIToken {
		return &models.User{Username: "api", Role: models.RoleAdmin, DisplayName: "API", IsAdmin: true, IsBoss: true}, true
	}
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "请先登录"})
	return nil, false
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	u, ok := h.requireUser(w, r)
	if !ok {
		return nil, false
	}
	if !h.authSvc.IsAdmin(u) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "仅管理员可操作"})
		return nil, false
	}
	return u, true
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.me)
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/deploy/active", h.deployActive)
	mux.HandleFunc("GET /api/deploy/snapshots", h.listDeploySnapshots)
	mux.HandleFunc("POST /api/deploy/rollback", h.rollbackDeploy)
	mux.HandleFunc("GET /api/releases", h.listReleases)
	mux.HandleFunc("POST /api/releases", h.createRelease)
	mux.HandleFunc("GET /api/releases/{id}", h.getRelease)
	mux.HandleFunc("POST /api/releases/{id}/submit-review", h.submitReview)
	mux.HandleFunc("POST /api/releases/{id}/boss-approve", h.bossApprove)
	mux.HandleFunc("POST /api/releases/{id}/deploy", h.deploy)
	mux.HandleFunc("POST /api/releases/{id}/cancel-deploy", h.cancelDeploy)
	mux.HandleFunc("POST /api/releases/{id}/switch", h.switchTraffic)
	mux.HandleFunc("POST /api/releases/{id}/verify", h.manualVerify)
	mux.HandleFunc("POST /api/releases/{id}/rollback", h.rollback)
	mux.HandleFunc("GET /api/traffic/status", h.trafficStatus)
	mux.HandleFunc("POST /api/traffic/to-green", h.trafficToGreen)
	mux.HandleFunc("POST /api/traffic/to-blue", h.trafficToBlue)
	mux.HandleFunc("GET /api/traffic/history", h.trafficHistory)
	mux.HandleFunc("GET /api/blue/active", h.blueActive)
	mux.HandleFunc("POST /api/blue/deploy", h.blueDeploy)
	mux.HandleFunc("POST /api/blue/sync", h.blueSync)
	mux.HandleFunc("POST /api/blue/sql/execute", h.executeBlueSQL)
	mux.HandleFunc("GET /api/component-sync/active", h.componentSyncActive)
	mux.HandleFunc("POST /api/component-sync/blue-to-green/all", h.componentSyncBlueToGreenAll)
	mux.HandleFunc("GET /api/components/specs", h.componentSpecs)
	mux.HandleFunc("GET /api/releases/{id}/executions", h.releaseExecutions)
	mux.HandleFunc("GET /api/releases/{id}/component-reports", h.releaseComponentReports)
	mux.HandleFunc("GET /api/releases/{id}/conflicts", h.releaseConflicts)
	mux.HandleFunc("POST /api/releases/{id}/conflicts", h.addReleaseConflict)
	mux.HandleFunc("POST /api/items/{itemId}/reviews", h.submitItemReview)
	mux.HandleFunc("POST /api/items/{itemId}/boss-approve", h.bossApproveItem)
	mux.HandleFunc("POST /api/items/{itemId}/rollback", h.rollbackItem)
	mux.HandleFunc("GET /api/migrations", h.listMigrations)
	mux.HandleFunc("GET /api/migrations/{id}", h.getMigrationSQL)
	mux.HandleFunc("POST /api/migrations/{id}/execute", h.executeMigration)
	mux.HandleFunc("POST /api/sql/execute", h.executeSQL)
	mux.HandleFunc("GET /api/components/auto-test/latest", h.componentAutoTestLatest)
	mux.HandleFunc("GET /api/components/auto-test/batches/{batchId}", h.componentAutoTestByBatch)
	mux.HandleFunc("POST /api/components/auto-test/run", h.componentAutoTestRun)
	mux.HandleFunc("POST /api/components/batch/apply", h.componentBatchApply)
	mux.HandleFunc("POST /api/components/{kind}/apply", h.componentApply)
	mux.HandleFunc("POST /api/components/{kind}/rollback", h.componentRollback)
	mux.HandleFunc("GET /api/components/{kind}/history", h.componentHistory)
	mux.HandleFunc("GET /api/releases/{id}/test-report", h.releaseTestReport)
	mux.HandleFunc("GET /api/admin/users", h.listUsers)
	mux.HandleFunc("POST /api/admin/users", h.createUserAdmin)
	mux.HandleFunc("PATCH /api/admin/users/{username}", h.updateUserAdmin)
	mux.HandleFunc("DELETE /api/admin/users/{username}", h.deleteUserAdmin)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	ghaEnabled := h.cfg.GitHubToken != "" &&
		(h.cfg.GitHubBackendRepo != "" || h.cfg.GitHubFrontendRepo != "")
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"mock_mode": h.cfg.MockMode,
		"deploy": map[string]any{
			"target":         "green",
			"green_url":      fmt.Sprintf("http://%s:28080/", h.cfg.ProdSSHHost),
			"prod_host":      h.cfg.ProdSSHHost,
			"prod_exec_mode": ssh.ResolvedExecMode(h.cfg),
			"gha_enabled":    ghaEnabled,
			"backend_repo":   h.cfg.GitHubBackendRepo,
			"frontend_repo":  h.cfg.GitHubFrontendRepo,
			"backend_ref":    h.cfg.GitHubBackendGitRef,
			"frontend_ref":   h.cfg.GitHubFrontendGitRef,
			"boss_reviewer":  h.cfg.BossReviewer,
			"super_admin":    h.cfg.SuperAdminUser,
		},
		"mysql": map[string]any{
			"green_container": h.cfg.GreenMySQLContainer,
			"green_database":  h.cfg.GreenMySQLDatabase,
			"blue_container":  h.cfg.BlueMySQLContainer,
			"blue_database":   h.cfg.BlueMySQLDatabase,
			"configured":      h.cfg.GreenMySQLRootPassword != "" && h.cfg.BlueMySQLRootPassword != "",
		},
	})
}

func (h *Handler) listReleases(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	list, err := h.release.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) createRelease(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req models.CreateReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Author == "" {
		req.Author = user.Username
	}
	rel, err := h.release.Create(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rel)
}

func (h *Handler) getRelease(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	rel, err := h.release.Get(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) submitReview(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "system"
	}
	rel, err := h.release.SubmitForReview(r.Context(), id, req.Actor)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) bossApprove(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var req models.BossApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Reviewer == "" {
		req.Reviewer = user.Username
	}
	rel, err := h.release.BossApprove(r.Context(), id, req, user.IsBoss)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) deployActive(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	active, err := h.release.GetActiveDeploy(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if active == nil {
		writeJSON(w, http.StatusOK, map[string]any{"busy": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"busy":   true,
		"id":     active.ID,
		"title":  active.Title,
		"status": active.Status,
	})
}

func (h *Handler) deploy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = user.Username
	}
	rel, err := h.release.StartDeploy(r.Context(), id, req.Actor, user.IsAdmin)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) cancelDeploy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = user.Username
	}
	rel, err := h.release.CancelDeploy(r.Context(), id, req.Actor)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) switchTraffic(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "ops"
	}
	rel, err := h.release.SwitchTraffic(r.Context(), id, req.Actor)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) manualVerify(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "owner"
	}
	rel, err := h.release.ConfirmManualVerify(r.Context(), id, req.Actor)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) rollback(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	var req models.ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "ops"
	}
	rel, err := h.release.Rollback(r.Context(), id, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) trafficStatus(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	st, err := h.traffic.Status(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) trafficToGreen(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "ops"
	}
	st, err := h.traffic.SwitchToGreen(r.Context(), req.Actor, req.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) trafficToBlue(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "ops"
	}
	st, err := h.traffic.SwitchToBlue(r.Context(), req.Actor, req.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *Handler) trafficHistory(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	list, err := h.traffic.History(r.Context(), 30)
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []models.SwitchEvent{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) submitItemReview(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	itemID := r.PathValue("itemId")
	var req models.SubmitReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	rel, err := h.release.SubmitReview(r.Context(), itemID, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) bossApproveItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	itemID := r.PathValue("itemId")
	var req models.ItemBossApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Reviewer == "" {
		req.Reviewer = user.Username
	}
	rel, err := h.release.BossApproveItem(r.Context(), itemID, req, user.IsBoss)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *Handler) rollbackItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Slot  string `json:"slot"`
		Actor string `json:"actor"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = user.Username
	}
	res, err := h.release.RollbackItem(r.Context(), r.PathValue("itemId"), req.Slot, req.Actor)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) listMigrations(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	list, err := h.migrate.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) executeMigration(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "ops"
	}
	res, err := h.migrate.Execute(r.Context(), id, req.Actor)
	if err != nil {
		if res != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "result": res})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) getMigrationSQL(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	sql, err := h.migrate.ReadSQL(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "sql": sql})
}

func (h *Handler) executeSQL(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	var req struct {
		SQL   string `json:"sql"`
		Actor string `json:"actor"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "ops"
	}
	res, err := h.migrate.ExecuteRaw(r.Context(), req.Label, req.SQL, req.Actor)
	if err != nil {
		if res != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "result": res})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) componentBatchApply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req componentops.BatchApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = user.Username
	}
	res, err := h.compOps.ApplyBatch(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "result": res})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) componentApply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req componentops.ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.Kind = r.PathValue("kind")
	if req.Actor == "" {
		req.Actor = user.Username
	}
	res, err := h.compOps.Apply(r.Context(), req)
	if err != nil {
		if res != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "result": res})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) componentRollback(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Slot  string `json:"slot"`
		Actor string `json:"actor"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	actor := req.Actor
	if actor == "" {
		actor = user.Username
	}
	slot := req.Slot
	if slot == "" {
		slot = "green"
	}
	res, err := h.compOps.Rollback(r.Context(), r.PathValue("kind"), slot, actor)
	if err != nil {
		if res != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "result": res})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) componentHistory(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	slot := r.URL.Query().Get("slot")
	if slot == "" {
		slot = "green"
	}
	list, err := h.compOps.ListHistory(r.Context(), r.PathValue("kind"), slot, 20)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) componentAutoTestLatest(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	report, err := h.release.Store().GetLatestBatchAutoTestReport(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) componentAutoTestByBatch(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	report, err := h.release.Store().GetBatchAutoTestReport(r.Context(), r.PathValue("batchId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) componentAutoTestRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req componentops.ManualAutoTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = user.Username
	}
	if req.Slot == "" {
		req.Slot = "green"
	}
	report, err := h.compOps.RunManualAutoTest(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) releaseTestReport(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	report, err := h.release.Store().GetLatestTestReport(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	list, err := h.authSvc.ListUsers(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) createUserAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var req models.AdminCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	u, err := h.authSvc.AdminCreateUser(r.Context(), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (h *Handler) updateUserAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var req models.AdminUpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	u, err := h.authSvc.AdminUpdateUser(r.Context(), r.PathValue("username"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *Handler) deleteUserAdmin(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	if err := h.authSvc.AdminDeleteUser(r.Context(), r.PathValue("username"), user.Username); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) blueActive(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, h.blue.Active())
}

func (h *Handler) blueDeploy(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "ops"
	}
	job, err := h.blue.StartDeploy(r.Context(), req.Actor, req.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) blueSync(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "ops"
	}
	job, err := h.blue.StartSync(r.Context(), req.Actor, req.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) componentSyncActive(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, h.sync.Active())
}

func (h *Handler) componentSyncBlueToGreenAll(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	var req models.ActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		req.Actor = "ops"
	}
	job, err := h.sync.StartBlueToGreenAll(r.Context(), req.Actor, req.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *Handler) componentSpecs(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	list, err := h.release.Store().ListComponentSpecs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) releaseExecutions(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	list, err := h.release.Store().ListChangeExecutions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) releaseComponentReports(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	list, err := h.release.Store().ListComponentTestReports(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) releaseConflicts(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	list, err := h.release.Store().ListConflictNotifications(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) addReleaseConflict(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req models.ConflictNotification
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	req.ReleaseID = r.PathValue("id")
	if req.Owner == "" {
		req.Owner = user.Username
	}
	if req.FilePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file_path required"})
		return
	}
	if h.cfg.SMTPHost != "" && req.Email != "" {
		if err := h.notify.SendConflict(r.Context(), req); err != nil {
			req.Status = "failed"
			req.Message = appendMessage(req.Message, "邮件发送失败: "+err.Error())
		} else {
			req.Status = "sent"
			req.Message = appendMessage(req.Message, "邮件已发送")
		}
	} else if req.Status == "" {
		req.Status = "audit_only"
	}
	if err := h.release.Store().AddConflictNotification(r.Context(), req); err != nil {
		writeErr(w, err)
		return
	}
	_ = h.release.Store().AddAudit(r.Context(), user.Username, "conflict_notification_recorded", req.ReleaseID, req.FilePath)
	writeJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
}

func (h *Handler) executeBlueSQL(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	if err := h.traffic.RequireProductionGreen(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	if h.blue.Active().Busy {
		writeErr(w, fmt.Errorf("蓝环境任务进行中，请等待完成后再执行 SQL"))
		return
	}
	var req struct {
		SQL   string `json:"sql"`
		Actor string `json:"actor"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "ops"
	}
	res, err := h.migrate.ExecuteRawBlue(r.Context(), req.Label, req.SQL, req.Actor)
	if err != nil {
		if res != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "result": res})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func appendMessage(base, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	if base == "" {
		return extra
	}
	if extra == "" {
		return base
	}
	return base + "\n" + extra
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	token, user, err := h.authSvc.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	user.IsAdmin = h.authSvc.IsAdmin(user)
	user.IsBoss = h.authSvc.IsBoss(user)
	writeJSON(w, http.StatusOK, models.LoginResponse{Token: token, User: *user})
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	token, user, err := h.authSvc.Register(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	user.IsAdmin = h.authSvc.IsAdmin(user)
	user.IsBoss = h.authSvc.IsBoss(user)
	writeJSON(w, http.StatusCreated, models.LoginResponse{Token: token, User: *user})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token := h.bearerToken(r)
	_ = h.authSvc.Logout(r.Context(), token)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) listDeploySnapshots(w http.ResponseWriter, r *http.Request) {
	if !h.auth(w, r) {
		return
	}
	target := r.URL.Query().Get("target")
	list, err := h.release.ListDeploySnapshots(r.Context(), target)
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []models.DeploySnapshot{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) rollbackDeploy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req models.DeployRollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = user.Username
	}
	snap, err := h.release.RollbackDeploy(r.Context(), req, user.IsAdmin)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}
