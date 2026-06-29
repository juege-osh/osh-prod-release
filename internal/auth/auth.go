package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/juege/osh-prod-release/internal/config"
	"github.com/juege/osh-prod-release/internal/models"
	"github.com/juege/osh-prod-release/internal/store"
)

const sessionHours = 72

type Service struct {
	store  *store.Store
	cfg    *config.Config
	pepper string
}

func New(cfg *config.Config, st *store.Store) *Service {
	pepper := cfg.AuthPepper
	if pepper == "" {
		pepper = "osh-prod-release"
	}
	return &Service{store: st, cfg: cfg, pepper: pepper}
}

func HashPassword(pepper, password string) string {
	sum := sha256.Sum256([]byte(pepper + ":" + password))
	return hex.EncodeToString(sum[:])
}

func (s *Service) SeedUsers(ctx context.Context) error {
	admin := strings.TrimSpace(s.cfg.SuperAdminUser)
	if admin == "" {
		admin = "juege"
	}
	pass := strings.TrimSpace(s.cfg.SuperAdminPassword)
	if pass == "" {
		pass = "juege123"
	}
	if err := s.store.EnsureUser(ctx, admin, HashPassword(s.pepper, pass), models.RoleAdmin, admin); err != nil {
		return err
	}
	for _, u := range s.cfg.AuthUsers {
		if u.Username == "" || u.Password == "" {
			continue
		}
		role := models.RoleNormal
		if strings.EqualFold(u.Username, admin) {
			role = models.RoleAdmin
		}
		if err := s.store.EnsureUser(ctx, u.Username, HashPassword(s.pepper, u.Password), role, u.DisplayName); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, *models.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return "", nil, fmt.Errorf("用户名和密码不能为空")
	}
	u, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		return "", nil, fmt.Errorf("用户名或密码错误")
	}
	if u.PasswordHash != HashPassword(s.pepper, password) {
		return "", nil, fmt.Errorf("用户名或密码错误")
	}
	token, err := newToken()
	if err != nil {
		return "", nil, err
	}
	expires := time.Now().UTC().Add(sessionHours * time.Hour)
	if err := s.store.CreateSession(ctx, token, u.Username, expires); err != nil {
		return "", nil, err
	}
	pub := u.Public()
	return token, &pub, nil
}

func (s *Service) Register(ctx context.Context, req models.RegisterRequest) (string, *models.User, error) {
	username := strings.TrimSpace(req.Username)
	displayName := strings.TrimSpace(req.DisplayName)
	if username == "" || req.Password == "" {
		return "", nil, fmt.Errorf("用户名和密码不能为空")
	}
	if len([]rune(username)) < 3 {
		return "", nil, fmt.Errorf("用户名至少需要 3 个字符")
	}
	if strings.ContainsAny(username, " \t\r\n") {
		return "", nil, fmt.Errorf("用户名不能包含空格")
	}
	if len([]rune(req.Password)) < 8 {
		return "", nil, fmt.Errorf("密码至少需要 8 个字符")
	}
	exists, err := s.store.UserExists(ctx, username)
	if err != nil {
		return "", nil, err
	}
	if exists {
		return "", nil, fmt.Errorf("用户名已存在")
	}
	if displayName == "" {
		displayName = username
	}
	if err := s.store.CreateUser(ctx, username, HashPassword(s.pepper, req.Password), models.RoleNormal, displayName); err != nil {
		return "", nil, err
	}
	return s.Login(ctx, username, req.Password)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, token)
}

func (s *Service) UserFromToken(ctx context.Context, token string) (*models.User, error) {
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	sess, err := s.store.GetSession(ctx, token)
	if err != nil {
		return nil, err
	}
	u, err := s.store.GetUserByUsername(ctx, sess.Username)
	if err != nil {
		return nil, err
	}
	pub := u.Public()
	return &pub, nil
}

func (s *Service) IsAdmin(u *models.User) bool {
	if u == nil {
		return false
	}
	return u.Role == models.RoleAdmin
}

func (s *Service) IsBoss(u *models.User) bool {
	if u == nil {
		return false
	}
	if u.Role == models.RoleAdmin {
		return true
	}
	boss := strings.TrimSpace(s.cfg.BossReviewer)
	if boss == "" {
		boss = "juege"
	}
	return strings.EqualFold(u.Username, boss)
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) ListUsers(ctx context.Context) ([]models.UserPublicRecord, error) {
	return s.store.ListUsers(ctx)
}

func (s *Service) AdminCreateUser(ctx context.Context, req models.AdminCreateUserRequest) (*models.UserPublicRecord, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		return nil, fmt.Errorf("username and password required")
	}
	exists, err := s.store.UserExists(ctx, username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("username already exists")
	}
	role := req.Role
	if role == "" {
		role = models.RoleNormal
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if err := s.store.CreateUser(ctx, username, HashPassword(s.pepper, req.Password), role, displayName); err != nil {
		return nil, err
	}
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.Username == username {
			return &u, nil
		}
	}
	return nil, fmt.Errorf("user created but not found")
}

func (s *Service) AdminUpdateUser(ctx context.Context, username string, req models.AdminUpdateUserRequest) (*models.UserPublicRecord, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username required")
	}
	if _, err := s.store.GetUserByUsername(ctx, username); err != nil {
		return nil, err
	}
	role := req.Role
	if role == "" {
		rec, err := s.store.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}
		role = rec.Role
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		rec, err := s.store.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, err
		}
		displayName = rec.DisplayName
	}
	if err := s.store.UpdateUser(ctx, username, role, displayName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Password) != "" {
		if err := s.store.UpdateUserPassword(ctx, username, HashPassword(s.pepper, req.Password)); err != nil {
			return nil, err
		}
	}
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.Username == username {
			return &u, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}

func (s *Service) AdminDeleteUser(ctx context.Context, username, actor string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username required")
	}
	admin := strings.TrimSpace(s.cfg.SuperAdminUser)
	if admin == "" {
		admin = "juege"
	}
	if strings.EqualFold(username, admin) {
		return fmt.Errorf("cannot delete super admin")
	}
	if strings.EqualFold(username, actor) {
		return fmt.Errorf("cannot delete yourself")
	}
	return s.store.DeleteUser(ctx, username)
}
