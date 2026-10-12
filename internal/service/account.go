package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) Initialize(input SetupInput) (*store.Site, error) {
	if err := validateSetup(input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Timezone) == "" {
		input.Timezone = "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(strings.TrimSpace(input.Timezone)); err != nil {
		return nil, fmt.Errorf("%w: 时区必须是有效的 IANA 名称", ErrValidation)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash administrator password: %w", err)
	}
	site := store.Site{
		Name:      strings.TrimSpace(input.SiteName),
		URL:       strings.TrimRight(strings.TrimSpace(input.SiteURL), "/"),
		AdminMail: strings.TrimSpace(input.AdminEmail),
		Timezone:  input.Timezone,
	}
	admin := store.Admin{
		Username:     input.AdminUsername,
		PasswordHash: string(hash),
		Email:        strings.TrimSpace(input.AdminEmail),
	}
	if err := s.repo.Setup(site, admin); err != nil {
		return nil, err
	}
	return &site, nil
}

func (s *Service) Authenticate(username, password string) (*store.Admin, error) {
	user, err := s.repo.User(username)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if user.Username != username || !user.Active || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return &user, nil
}

func (s *Service) Admin() (*store.Admin, error) {
	admin, err := s.repo.Admin()
	if err != nil {
		return nil, err
	}
	admin.Role = "owner"
	admin.Active = true
	return admin, nil
}

func (s *Service) User(username string) (*store.User, error) {
	user, err := s.repo.User(username)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) UpdateAdminAccount(input AccountInput) (*store.Admin, error) {
	admin, err := s.repo.Admin()
	if err != nil {
		return nil, err
	}

	email := strings.TrimSpace(input.Email)
	if email == "" {
		email = admin.Email
	}
	if !validEmail(email) {
		return nil, fmt.Errorf("%w: 邮箱格式不正确", ErrValidation)
	}

	passwordHash := admin.PasswordHash
	if input.NewPassword != "" {
		if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.CurrentPassword)) != nil {
			return nil, ErrInvalidCredentials
		}
		if len(input.NewPassword) < 8 {
			return nil, fmt.Errorf("%w: 新密码至少 8 个字符", ErrValidation)
		}
		passwordHashBytes, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash administrator password: %w", err)
		}
		passwordHash = string(passwordHashBytes)
	}

	admin.Email = email
	admin.PasswordHash = passwordHash
	if err := s.repo.UpdateAdmin(*admin); err != nil {
		return nil, err
	}
	if input.NewPassword != "" {
		if err := s.RevokeOtherSessions(admin.Username, input.SessionToken); err != nil {
			return nil, err
		}
	}
	admin.Role = "owner"
	admin.Active = true
	return admin, nil
}

func (s *Service) ListUsers() ([]store.User, error) {
	return s.repo.Users()
}

func (s *Service) CreateUser(input UserInput) (*store.User, error) {
	if input.Role == "" {
		input.Role = "editor"
	}
	if err := validateUserInput(input, true); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash user password: %w", err)
	}
	user, err := s.repo.CreateUser(store.User{Username: input.Username, Email: strings.TrimSpace(input.Email), PasswordHash: string(hash), Role: input.Role, Active: true})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) UpdateUser(username string, input UserInput) (*store.User, error) {
	user, err := s.repo.User(username)
	if err != nil {
		return nil, err
	}
	if user.Role == "owner" {
		return nil, fmt.Errorf("%w: 所有者账户请在账户设置中修改", ErrValidation)
	}
	if input.Email != "" {
		if !validEmail(input.Email) {
			return nil, fmt.Errorf("%w: 邮箱格式不正确", ErrValidation)
		}
		user.Email = strings.TrimSpace(input.Email)
	}
	if input.Role != "" {
		if !validStaffRole(input.Role) {
			return nil, fmt.Errorf("%w: 角色只能是编辑或审核员", ErrValidation)
		}
		user.Role = input.Role
	}
	if input.Active != nil {
		user.Active = *input.Active
	}
	if input.Password != "" {
		if len(input.Password) < 8 {
			return nil, fmt.Errorf("%w: 密码至少 8 个字符", ErrValidation)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash user password: %w", err)
		}
		user.PasswordHash = string(hash)
	}
	updated, err := s.repo.UpdateUser(user)
	if err != nil {
		return nil, err
	}
	if input.Password != "" || (input.Active != nil && !*input.Active) {
		if err := s.RevokeUserSessions(username); err != nil {
			return nil, err
		}
	}
	return &updated, nil
}

func (s *Service) DeleteUser(username string) error {
	user, err := s.repo.User(username)
	if err != nil {
		return err
	}
	if user.Role == "owner" {
		return fmt.Errorf("%w: 不能删除所有者账户", ErrValidation)
	}
	if err := s.RevokeUserSessions(username); err != nil {
		return err
	}
	return s.repo.DeleteUser(username)
}

func (s *Service) Site() (*store.Site, error) {
	return s.repo.Site()
}

func (s *Service) UpdateSite(site store.Site) (*store.Site, error) {
	if strings.TrimSpace(site.Name) == "" || !validURL(site.URL) || !validEmail(site.AdminMail) {
		return nil, fmt.Errorf("%w: 需要站点名称、有效地址和有效的管理员邮箱", ErrValidation)
	}
	current, err := s.repo.Site()
	if err != nil {
		return nil, err
	}
	description := strings.TrimSpace(site.Description)
	keywords := strings.TrimSpace(site.Keywords)
	if runeLength(description) > 2000 {
		return nil, fmt.Errorf("%w: 描述最多 2000 个字符", ErrValidation)
	}
	if runeLength(keywords) > 500 {
		return nil, fmt.Errorf("%w: 关键词最多 500 个字符", ErrValidation)
	}
	current.Name = strings.TrimSpace(site.Name)
	current.URL = strings.TrimRight(strings.TrimSpace(site.URL), "/")
	current.AdminMail = strings.TrimSpace(site.AdminMail)
	current.Description = description
	current.Keywords = keywords
	if timezone := strings.TrimSpace(site.Timezone); timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			return nil, fmt.Errorf("%w: 时区必须是有效的 IANA 名称", ErrValidation)
		}
		current.Timezone = timezone
	}
	if err := s.repo.UpdateSite(*current); err != nil {
		return nil, err
	}
	return current, nil
}
