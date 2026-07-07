package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type AuthUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
	Legacy      bool   `json:"legacy,omitempty"`
}

type UserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	Active      *bool  `json:"active,omitempty"`
}

type PasswordInput struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type UserStore struct {
	db *sql.DB
}

func OpenUserStore(ctx context.Context, cfg Config) (*UserStore, error) {
	if cfg.DBDSN == "" {
		return nil, nil
	}
	db, err := sql.Open("mysql", cfg.DBDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &UserStore{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.ensureBootstrapUser(ctx, cfg); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *UserStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *UserStore) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS peapod_users (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(64) NOT NULL,
			display_name VARCHAR(80) NOT NULL DEFAULT '',
			email VARCHAR(160) NOT NULL DEFAULT '',
			password_hash VARCHAR(255) NOT NULL,
			role VARCHAR(24) NOT NULL DEFAULT 'operator',
			active TINYINT(1) NOT NULL DEFAULT 1,
			last_login_at DATETIME(3) NULL,
			created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
			UNIQUE KEY uk_peapod_users_username (username),
			KEY idx_peapod_users_role (role),
			KEY idx_peapod_users_active (active)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS peapod_deploy_audit_logs (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
			user_id BIGINT NULL,
			username VARCHAR(64) NOT NULL DEFAULT '',
			remote_ip VARCHAR(128) NOT NULL DEFAULT '',
			task_id VARCHAR(128) NOT NULL DEFAULT '',
			task_title VARCHAR(160) NOT NULL DEFAULT '',
			repo_id INT NOT NULL DEFAULT 0,
			branch VARCHAR(128) NOT NULL DEFAULT '',
			variables_json TEXT NOT NULL,
			pipeline_number BIGINT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT '',
			error TEXT NULL,
			KEY idx_peapod_audit_created_at (created_at),
			KEY idx_peapod_audit_user_id (user_id),
			KEY idx_peapod_audit_task_id (task_id),
			KEY idx_peapod_audit_status (status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return s.copyLegacyTables(ctx)
}

func (s *UserStore) copyLegacyTables(ctx context.Context) error {
	if err := s.copyLegacyUsers(ctx); err != nil {
		return err
	}
	return s.copyLegacyAuditLogs(ctx)
}

func (s *UserStore) copyLegacyUsers(ctx context.Context) error {
	legacy, err := s.tableExists(ctx, "zephyr_users")
	if err != nil || !legacy {
		return err
	}
	count, err := s.tableRowCount(ctx, "peapod_users")
	if err != nil || count > 0 {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO peapod_users
		(id, username, display_name, email, password_hash, role, active, last_login_at, created_at, updated_at)
		SELECT id, username, display_name, email, password_hash, role, active, last_login_at, created_at, updated_at
		FROM zephyr_users`)
	return err
}

func (s *UserStore) copyLegacyAuditLogs(ctx context.Context) error {
	legacy, err := s.tableExists(ctx, "zephyr_deploy_audit_logs")
	if err != nil || !legacy {
		return err
	}
	count, err := s.tableRowCount(ctx, "peapod_deploy_audit_logs")
	if err != nil || count > 0 {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO peapod_deploy_audit_logs
		(id, created_at, user_id, username, remote_ip, task_id, task_title, repo_id, branch, variables_json, pipeline_number, status, error)
		SELECT id, created_at, user_id, username, remote_ip, task_id, task_title, repo_id, branch, variables_json, pipeline_number, status, error
		FROM zephyr_deploy_audit_logs`)
	return err
}

func (s *UserStore) tableExists(ctx context.Context, name string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`, name).Scan(&count)
	return count > 0, err
}

func (s *UserStore) tableRowCount(ctx context.Context, name string) (int, error) {
	switch name {
	case "peapod_users", "peapod_deploy_audit_logs":
	default:
		return 0, errors.New("unsupported table")
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+name).Scan(&count)
	return count, err
}

func (s *UserStore) ensureBootstrapUser(ctx context.Context, cfg Config) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM peapod_users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	username := normalizeUsername(cfg.BootstrapUsername)
	if username == "" {
		username = "admin"
	}
	password := cfg.BootstrapPassword
	if password == "" {
		password = cfg.Password
	}
	if password == "" {
		return errors.New("PEAPOD_BOOTSTRAP_PASSWORD or PEAPOD_PASSWORD is required when PEAPOD_DB_DSN initializes an empty database")
	}
	displayName := strings.TrimSpace(cfg.BootstrapDisplayName)
	if displayName == "" {
		displayName = "管理员"
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO peapod_users (username, display_name, email, password_hash, role, active) VALUES (?, ?, ?, ?, 'admin', 1)`,
		username, displayName, strings.TrimSpace(cfg.BootstrapEmail), hash)
	return err
}

func (s *UserStore) Authenticate(ctx context.Context, login, password string) (AuthUser, error) {
	login = normalizeLogin(login)
	if login == "" || password == "" {
		return AuthUser{}, errors.New("用户名或密码不正确")
	}
	user, hash, err := s.getUserWithHash(ctx, login)
	if err != nil {
		return AuthUser{}, errors.New("用户名或密码不正确")
	}
	if !user.Active {
		return AuthUser{}, errors.New("账号已停用")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return AuthUser{}, errors.New("用户名或密码不正确")
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE peapod_users SET last_login_at = CURRENT_TIMESTAMP(3) WHERE id = ?`, user.ID)
	return user, nil
}

func (s *UserStore) GetUser(ctx context.Context, id int64) (AuthUser, error) {
	var user AuthUser
	err := s.db.QueryRowContext(ctx, `SELECT id, username, display_name, email, role, active FROM peapod_users WHERE id = ?`, id).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.Role, &user.Active)
	if err != nil {
		return AuthUser{}, err
	}
	return user, nil
}

func (s *UserStore) ListUsers(ctx context.Context) ([]AuthUser, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, display_name, email, role, active FROM peapod_users ORDER BY active DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []AuthUser{}
	for rows.Next() {
		var user AuthUser
		if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.Role, &user.Active); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *UserStore) CreateUser(ctx context.Context, input UserInput) (AuthUser, error) {
	username := normalizeUsername(input.Username)
	if username == "" {
		return AuthUser{}, errors.New("用户名不能为空")
	}
	email := normalizeEmail(input.Email)
	if email != "" {
		exists, err := s.emailExists(ctx, email, 0)
		if err != nil {
			return AuthUser{}, err
		}
		if exists {
			return AuthUser{}, errors.New("邮箱已被使用")
		}
	}
	if input.Password == "" {
		return AuthUser{}, errors.New("初始密码不能为空")
	}
	role := normalizeRole(input.Role)
	hash, err := hashPassword(input.Password)
	if err != nil {
		return AuthUser{}, err
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO peapod_users (username, display_name, email, password_hash, role, active) VALUES (?, ?, ?, ?, ?, ?)`,
		username, strings.TrimSpace(input.DisplayName), email, hash, role, boolToInt(active))
	if err != nil {
		return AuthUser{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetUser(ctx, id)
}

func (s *UserStore) UpdateUser(ctx context.Context, id int64, input UserInput) (AuthUser, error) {
	if id <= 0 {
		return AuthUser{}, errors.New("用户不存在")
	}
	current, err := s.GetUser(ctx, id)
	if err != nil {
		return AuthUser{}, err
	}
	username := normalizeUsername(input.Username)
	if username == "" {
		username = current.Username
	}
	displayName := strings.TrimSpace(input.DisplayName)
	email := normalizeEmail(input.Email)
	if email != "" {
		exists, err := s.emailExists(ctx, email, id)
		if err != nil {
			return AuthUser{}, err
		}
		if exists {
			return AuthUser{}, errors.New("邮箱已被使用")
		}
	}
	role := normalizeRole(input.Role)
	active := current.Active
	if input.Active != nil {
		active = *input.Active
	}
	_, err = s.db.ExecContext(ctx, `UPDATE peapod_users SET username = ?, display_name = ?, email = ?, role = ?, active = ? WHERE id = ?`,
		username, displayName, email, role, boolToInt(active), id)
	if err != nil {
		return AuthUser{}, err
	}
	return s.GetUser(ctx, id)
}

func (s *UserStore) SetPassword(ctx context.Context, id int64, password string) error {
	if id <= 0 {
		return errors.New("用户不存在")
	}
	if password == "" {
		return errors.New("新密码不能为空")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE peapod_users SET password_hash = ? WHERE id = ?`, hash, id)
	return err
}

func (s *UserStore) VerifyPassword(ctx context.Context, id int64, password string) error {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM peapod_users WHERE id = ? AND active = 1`, id).Scan(&hash)
	if err != nil {
		return errors.New("账号不存在或已停用")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return errors.New("旧密码不正确")
	}
	return nil
}

func (s *UserStore) WriteAudit(ctx context.Context, record AuditRecord) error {
	payload, _ := json.Marshal(record.Variables)
	_, err := s.db.ExecContext(ctx, `INSERT INTO peapod_deploy_audit_logs
		(user_id, username, remote_ip, task_id, task_title, repo_id, branch, variables_json, pipeline_number, status, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullableUserID(record.UserID), record.Username, record.RemoteIP, record.TaskID, record.TaskTitle, record.RepoID, record.Branch, string(payload), record.Pipeline, record.Status, nullableString(record.Error))
	return err
}

func (s *UserStore) ListAudit(ctx context.Context, limit int) ([]AuditRecord, error) {
	if limit <= 0 {
		limit = 80
	}
	rows, err := s.db.QueryContext(ctx, `SELECT created_at, user_id, username, remote_ip, task_id, task_title, repo_id, branch, variables_json, pipeline_number, status, COALESCE(error, '')
		FROM peapod_deploy_audit_logs
		ORDER BY created_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []AuditRecord{}
	for rows.Next() {
		var createdAt time.Time
		var userID sql.NullInt64
		var variablesJSON string
		var record AuditRecord
		if err := rows.Scan(&createdAt, &userID, &record.Username, &record.RemoteIP, &record.TaskID, &record.TaskTitle, &record.RepoID, &record.Branch, &variablesJSON, &record.Pipeline, &record.Status, &record.Error); err != nil {
			return nil, err
		}
		if userID.Valid {
			record.UserID = userID.Int64
		}
		record.Time = createdAt.Format(time.RFC3339)
		record.Variables = map[string]string{}
		_ = json.Unmarshal([]byte(variablesJSON), &record.Variables)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *UserStore) getUserWithHash(ctx context.Context, login string) (AuthUser, string, error) {
	var user AuthUser
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT id, username, display_name, email, role, active, password_hash
		FROM peapod_users
		WHERE username = ? OR LOWER(email) = ?
		ORDER BY CASE WHEN username = ? THEN 0 ELSE 1 END
		LIMIT 1`, login, login, login).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.Email, &user.Role, &user.Active, &hash)
	if err != nil {
		return AuthUser{}, "", err
	}
	return user, hash, nil
}

func (s *UserStore) emailExists(ctx context.Context, email string, excludeID int64) (bool, error) {
	if email == "" {
		return false, nil
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM peapod_users WHERE LOWER(email) = ? AND id <> ?`, email, excludeID).Scan(&count)
	return count > 0, err
}

func hashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("密码至少需要 8 位")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeLogin(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "@") {
		return normalizeEmail(value)
	}
	return normalizeUsername(value)
}

func normalizeRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "admin":
		return "admin"
	default:
		return "operator"
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableUserID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
