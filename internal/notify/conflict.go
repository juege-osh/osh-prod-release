package notify

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
)

type sendMailFunc func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error

type Service struct {
	cfg      *config.Config
	sendMail sendMailFunc
}

func New(cfg *config.Config) *Service {
	return &Service{cfg: cfg, sendMail: smtp.SendMail}
}

func NewWithSender(cfg *config.Config, sender sendMailFunc) *Service {
	return &Service{cfg: cfg, sendMail: sender}
}

func (s *Service) SendConflict(ctx context.Context, n models.ConflictNotification) error {
	if s == nil || s.cfg == nil {
		return fmt.Errorf("notifier not configured")
	}
	to := strings.TrimSpace(n.Email)
	if to == "" {
		return fmt.Errorf("conflict owner email is empty")
	}
	host := strings.TrimSpace(s.cfg.SMTPHost)
	if host == "" {
		return fmt.Errorf("SMTP_HOST not configured")
	}
	from := strings.TrimSpace(s.cfg.SMTPFrom)
	if from == "" {
		from = s.cfg.SMTPUser
	}
	if from == "" {
		from = "osh-prod-release@localhost"
	}
	port := s.cfg.SMTPPort
	if port <= 0 {
		port = 25
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	var auth smtp.Auth
	if s.cfg.SMTPUser != "" || s.cfg.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPassword, host)
	}
	msg := BuildConflictEmail(from, to, n)
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.sendMail(addr, auth, from, []string{to}, []byte(msg))
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("send conflict email timeout")
	}
}

func BuildConflictEmail(from, to string, n models.ConflictNotification) string {
	subject := fmt.Sprintf("[OSH发布冲突] %s", defaultString(n.FilePath, n.ReleaseID))
	body := strings.Join([]string{
		"检测到上线冲突，请对应开发者确认。",
		"",
		"发布单: " + n.ReleaseID,
		"上线项: " + n.ItemID,
		"冲突文件: " + n.FilePath,
		"责任人: " + n.Owner,
		"说明: " + n.Message,
		"",
		"请在发布控制台 Change 作战台处理后继续上线。",
	}, "\n")
	return strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(fallback)
}
