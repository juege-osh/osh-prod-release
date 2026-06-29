package component

import (
	"context"
	"testing"

	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
)

func TestRegisteredDataComponentsUseScriptExecutor(t *testing.T) {
	svc := NewService(&config.Config{MockMode: true}, nil)
	rel := models.Release{ID: "rel-test"}

	for _, kind := range []string{"mysql", "redis", "es", "kafka", "nacos", "hbase", "mongodb"} {
		item := models.ChangeItem{
			ID:            "item-" + kind,
			Type:          models.ItemTypeComponent,
			Component:     kind,
			ComponentType: kind,
		}
		ex := svc.ExecutorFor(item)
		if ex == nil {
			t.Fatalf("ExecutorFor(%s) returned nil", kind)
		}
		res, err := ex.ApplyGreen(context.Background(), rel, item)
		if err != nil {
			t.Fatalf("ApplyGreen(%s) returned error: %v", kind, err)
		}
		if res.Status != "success" {
			t.Fatalf("ApplyGreen(%s) status = %q, want success", kind, res.Status)
		}
		if res.Message == "placeholder executor" {
			t.Fatalf("ApplyGreen(%s) used placeholder executor", kind)
		}
	}
}

func TestUnknownComponentRemainsExtensionPlaceholder(t *testing.T) {
	svc := NewService(&config.Config{MockMode: true}, nil)
	item := models.ChangeItem{
		ID:            "item-custom",
		Type:          models.ItemTypeComponent,
		Component:     "custom",
		ComponentType: "custom",
	}

	ex := svc.ExecutorFor(item)
	if ex == nil {
		t.Fatal("ExecutorFor(custom) returned nil")
	}
	res, err := ex.ApplyGreen(context.Background(), models.Release{ID: "rel-test"}, item)
	if err != nil {
		t.Fatalf("ApplyGreen(custom) returned error: %v", err)
	}
	if res.Status != "skipped" {
		t.Fatalf("ApplyGreen(custom) status = %q, want skipped", res.Status)
	}
}
