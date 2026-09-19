package services

import (
	"errors"
	"strings"
	"sync"

	"dezuxk/internal/config"
	"dezuxk/internal/db"
	"dezuxk/internal/models"

	"golang.org/x/crypto/bcrypt"
)

// AuthService handles authentication, user management, and credential updates.
type AuthService struct {
	database   *db.DB
	registerMu sync.Mutex
}

// NewAuthService creates a new AuthService instance.
func NewAuthService(database *db.DB) *AuthService {
	return &AuthService{database: database}
}

// Login verifies credentials and returns the authenticated user info.
// If username is empty, it authenticates against the primary admin account.
func (s *AuthService) Login(username, password string) (*models.UserResponse, error) {
	username = strings.TrimSpace(username)
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("Vui lòng nhập mật khẩu")
	}

	var user *models.User
	var err error

	if username == "" {
		user, err = s.database.GetAdminUser()
	} else {
		user, err = s.database.GetUserByUsername(username)
	}

	if err != nil {
		return nil, errors.New("Mật khẩu không chính xác")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("Mật khẩu không chính xác")
	}

	return &models.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}, nil
}

// ChangePassword updates the user's password after verifying their current password.
func (s *AuthService) ChangePassword(userID int64, currentPassword, newPassword string) error {
	if strings.TrimSpace(currentPassword) == "" {
		return errors.New("Vui lòng nhập mật khẩu hiện tại")
	}
	if len(strings.TrimSpace(newPassword)) < config.MinPasswordLength {
		return errors.New("Mật khẩu mới phải có ít nhất 5 ký tự")
	}

	var user *models.User
	if userID <= 0 {
		return errors.New("chưa xác định người dùng hiện tại")
	}
	user, err := s.database.GetUserByID(userID)

	if err != nil || user == nil {
		return errors.New("Không tìm thấy tài khoản người dùng")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return errors.New("Mật khẩu hiện tại không chính xác")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("Không thể mã hoá mật khẩu mới")
	}

	return s.database.UpdateUserPassword(user.ID, string(hashed))
}

// Register registers a new user into SQLite database.
func (s *AuthService) Register(username, password string) (*models.UserResponse, error) {
	s.registerMu.Lock()
	defer s.registerMu.Unlock()

	username = strings.TrimSpace(username)
	if len(username) < 3 {
		return nil, errors.New("Tên đăng nhập phải có ít nhất 3 ký tự")
	}
	if len(password) < config.MinPasswordLength {
		return nil, errors.New("Mật khẩu phải có ít nhất 5 ký tự")
	}

	userCount, err := s.database.CountUsers()
	if err != nil {
		return nil, errors.New("Không thể kiểm tra trạng thái thiết lập tài khoản")
	}
	if userCount > 0 {
		return nil, errors.New("Đăng ký quản trị viên chỉ được phép trong lần thiết lập đầu tiên")
	}

	existing, _ := s.database.GetUserByUsername(username)
	if existing != nil {
		return nil, errors.New("Tên đăng nhập đã tồn tại")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("Lỗi tạo mật khẩu")
	}

	user, err := s.database.CreateUser(username, string(hashed), "admin")
	if err != nil {
		return nil, errors.New("Lỗi tạo tài khoản")
	}

	return &models.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}, nil
}
