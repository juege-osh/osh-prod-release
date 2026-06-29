package componentops

import "testing"

func TestNormalizeKind(t *testing.T) {
	if normalizeKind("elasticsearch") != "es" {
		t.Fatalf("expected es")
	}
	if defaultAction("kafka") != "create-topic" {
		t.Fatalf("expected create-topic")
	}
}
