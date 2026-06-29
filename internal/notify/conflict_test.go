package notify

import (
	"context"
	"net/smtp"
	"strings"
	"testing"

	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
)

func TestSendConflictUsesSMTPConfigAndConflictPayload(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg string
	svc := NewWithSender(&config.Config{
		SMTPHost: "smtp.example.com",
		SMTPPort: 2525,
		SMTPUser: "ops@example.com",
		SMTPFrom: "release@example.com",
	}, func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
		gotAddr = addr
		gotFrom = from
		gotTo = append([]string(nil), to...)
		gotMsg = string(msg)
		return nil
	})

	err := svc.SendConflict(context.Background(), models.ConflictNotification{
		ReleaseID: "rel-1",
		ItemID:    "item-1",
		FilePath:  "/www/ruoyi/pom.xml",
		Owner:     "alice",
		Email:     "alice@example.com",
		Message:   "同一文件存在多人修改",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAddr != "smtp.example.com:2525" {
		t.Fatalf("addr = %q", gotAddr)
	}
	if gotFrom != "release@example.com" {
		t.Fatalf("from = %q", gotFrom)
	}
	if len(gotTo) != 1 || gotTo[0] != "alice@example.com" {
		t.Fatalf("to = %#v", gotTo)
	}
	for _, want := range []string{"[OSH发布冲突]", "/www/ruoyi/pom.xml", "alice", "同一文件存在多人修改"} {
		if !strings.Contains(gotMsg, want) {
			t.Fatalf("message missing %q:\n%s", want, gotMsg)
		}
	}
}
