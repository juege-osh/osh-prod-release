package release

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/juege/osh-prod-release/internal/component"
	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
	"github.com/juege/osh-prod-release/internal/store"
)

func TestExecuteComponentChangesRollsBackAppliedComponentsInReverseOrder(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	rel, err := st.CreateRelease(ctx, models.CreateReleaseRequest{
		Title:        "component rollback",
		Level:        models.LevelNormal,
		Author:       "tester",
		DeployTarget: "green",
		Items: []models.CreateChangeItemRequest{
			{Title: "redis", Type: models.ItemTypeComponent, Component: "redis", ComponentType: "redis", DeployOrder: 1},
			{Title: "kafka", Type: models.ItemTypeComponent, Component: "kafka", ComponentType: "kafka", DeployOrder: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var calls []string
	redisExec := &recordingComponentExecutor{kind: "redis", calls: &calls}
	kafkaExec := &recordingComponentExecutor{kind: "kafka", calls: &calls, testErr: errors.New("kafka smoke failed")}
	svc := New(&config.Config{MockMode: true, BossReviewer: "juege"}, st, nil)
	svc.component.RegisterExecutor("redis", redisExec)
	svc.component.RegisterExecutor("kafka", kafkaExec)

	_, err = svc.executeComponentChanges(ctx, *rel, "green", "tester")
	if err == nil {
		t.Fatal("executeComponentChanges succeeded, want kafka test failure")
	}

	want := []string{
		"redis:snapshot",
		"redis:apply-green",
		"redis:test",
		"kafka:snapshot",
		"kafka:apply-green",
		"kafka:test",
		"kafka:rollback-green",
		"redis:rollback-green",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestRollbackItemRunsGreenComponentRollback(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	rel, err := st.CreateRelease(ctx, models.CreateReleaseRequest{
		Title:        "manual rollback",
		Level:        models.LevelNormal,
		Author:       "tester",
		DeployTarget: "green",
		Items: []models.CreateChangeItemRequest{
			{Title: "redis key", Type: models.ItemTypeComponent, Component: "redis", ComponentType: "redis", DeployOrder: 1, TargetNode: "node-a"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var calls []string
	redisExec := &recordingComponentExecutor{kind: "redis", calls: &calls}
	svc := New(&config.Config{MockMode: true, BossReviewer: "juege"}, st, nil)
	svc.component.RegisterExecutor("redis", redisExec)

	res, err := svc.RollbackItem(ctx, rel.Items[0].ID, "green", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "success" || res.Phase != component.PhaseRollbackGreen {
		t.Fatalf("rollback result = %#v", res)
	}
	want := []string{"redis:rollback-green"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

type recordingComponentExecutor struct {
	kind    string
	calls   *[]string
	testErr error
}

func (e *recordingComponentExecutor) Kind() string { return e.kind }

func (e *recordingComponentExecutor) Plan(context.Context, models.Release, models.ChangeItem) (component.Result, error) {
	return e.result(component.PhasePlan, "green", "planned"), nil
}

func (e *recordingComponentExecutor) Snapshot(context.Context, models.Release, models.ChangeItem, string) (component.Result, error) {
	*e.calls = append(*e.calls, e.kind+":snapshot")
	return e.result(component.PhaseSnapshot, "green", "success"), nil
}

func (e *recordingComponentExecutor) ApplyGreen(context.Context, models.Release, models.ChangeItem) (component.Result, error) {
	*e.calls = append(*e.calls, e.kind+":apply-green")
	return e.result(component.PhaseApplyGreen, "green", "success"), nil
}

func (e *recordingComponentExecutor) Test(context.Context, models.Release, models.ChangeItem, string) (component.Result, error) {
	*e.calls = append(*e.calls, e.kind+":test")
	res := e.result(component.PhaseTest, "green", "success")
	if e.testErr != nil {
		res.Status = "failed"
	}
	return res, e.testErr
}

func (e *recordingComponentExecutor) RollbackGreen(context.Context, models.Release, models.ChangeItem) (component.Result, error) {
	*e.calls = append(*e.calls, e.kind+":rollback-green")
	return e.result(component.PhaseRollbackGreen, "green", "success"), nil
}

func (e *recordingComponentExecutor) ApplyBlue(context.Context, models.Release, models.ChangeItem) (component.Result, error) {
	return e.result(component.PhaseApplyBlue, "blue", "success"), nil
}

func (e *recordingComponentExecutor) RollbackBlue(context.Context, models.Release, models.ChangeItem) (component.Result, error) {
	return e.result(component.PhaseRollbackBlue, "blue", "success"), nil
}

func (e *recordingComponentExecutor) result(phase component.Phase, slot, status string) component.Result {
	return component.Result{
		Phase:     phase,
		Slot:      slot,
		Component: e.kind,
		Action:    "apply",
		Status:    status,
		Message:   string(phase),
	}
}
