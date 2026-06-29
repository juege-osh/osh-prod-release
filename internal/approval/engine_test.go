package approval

import (
	"strings"
	"testing"

	"github.com/juege/osh-prod-release/internal/models"
)

func TestItemReviewOKAllowsDeveloperReviewerWhenPeerSawDemo(t *testing.T) {
	engine := New("juege")
	item := models.ChangeItem{
		Title:        "支付回调修复",
		Developer:    "alice",
		Reviewer1:    "alice",
		Reviewer2:    "bob",
		DemoRequired: true,
		Reviews: []models.Review{
			{Reviewer: "alice", Tested: true, Result: models.ReviewApprove},
			{Reviewer: "bob", Tested: true, DemoSeen: true, Result: models.ReviewApprove},
		},
	}

	ok, msg := engine.ItemReviewOK(item)
	if !ok {
		t.Fatalf("ItemReviewOK rejected valid developer+peer review: %s", msg)
	}
}

func TestItemReviewOKRequiresPeerDemoSeenForDeveloperReviewer(t *testing.T) {
	engine := New("juege")
	item := models.ChangeItem{
		Title:        "支付回调修复",
		Developer:    "alice",
		Reviewer1:    "alice",
		Reviewer2:    "bob",
		DemoRequired: true,
		Reviews: []models.Review{
			{Reviewer: "alice", Tested: true, Result: models.ReviewApprove},
			{Reviewer: "bob", Tested: true, Result: models.ReviewApprove},
		},
	}

	ok, msg := engine.ItemReviewOK(item)
	if ok {
		t.Fatal("ItemReviewOK accepted review without peer demo confirmation")
	}
	if !strings.Contains(msg, "观看开发者演示") {
		t.Fatalf("message = %q, want demo guidance", msg)
	}
}
