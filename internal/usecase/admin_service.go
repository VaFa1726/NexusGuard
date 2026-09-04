package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nexusguard/bot/internal/repository/postgres"
)

// AdminService handles bot-role management for groups.
type AdminService struct {
	adminRepo *postgres.AdminRepository
	groupRepo *postgres.GroupRepository
}

func NewAdminService(ar *postgres.AdminRepository, gr *postgres.GroupRepository) *AdminService {
	return &AdminService{adminRepo: ar, groupRepo: gr}
}

// GrantOwner sets a user as owner of the group in the bot system.
// Called automatically when the bot is added to a group.
func (s *AdminService) GrantOwner(ctx context.Context, groupDBID, telegramID int64, username string) error {
	err := s.adminRepo.SetRole(ctx, groupDBID, telegramID, telegramID, username, postgres.RoleOwner)
	if err != nil {
		return fmt.Errorf("GrantOwner: %w", err)
	}
	slog.Info("Owner granted", "group_db_id", groupDBID, "telegram_id", telegramID)
	return nil
}

// AddAdmin grants admin role. Only owner can do this.
func (s *AdminService) AddAdmin(ctx context.Context, groupDBID, granterID, targetID int64, targetUsername string) error {
	// Check granter is owner
	role, _ := s.adminRepo.GetRole(ctx, groupDBID, granterID)
	if role != postgres.RoleOwner {
		return fmt.Errorf("only owner can add admins")
	}
	return s.adminRepo.SetRole(ctx, groupDBID, targetID, granterID, targetUsername, postgres.RoleAdmin)
}

// AddModerator grants moderator role. Owner or Admin can do this.
func (s *AdminService) AddModerator(ctx context.Context, groupDBID, granterID, targetID int64, targetUsername string) error {
	role, _ := s.adminRepo.GetRole(ctx, groupDBID, granterID)
	if !postgres.HasMinRole(role, postgres.RoleAdmin) {
		return fmt.Errorf("only admins and owners can add moderators")
	}
	return s.adminRepo.SetRole(ctx, groupDBID, targetID, granterID, targetUsername, postgres.RoleModerator)
}

// RemoveRole revokes a user's bot role. Only owner can remove.
func (s *AdminService) RemoveRole(ctx context.Context, groupDBID, granterID, targetID int64) error {
	role, _ := s.adminRepo.GetRole(ctx, groupDBID, granterID)
	if role != postgres.RoleOwner {
		return fmt.Errorf("only owner can remove roles")
	}
	return s.adminRepo.RemoveRole(ctx, groupDBID, targetID)
}

// ListAdmins returns all bot admins for a group.
func (s *AdminService) ListAdmins(ctx context.Context, groupDBID int64) ([]postgres.BotAdmin, error) {
	return s.adminRepo.ListAdmins(ctx, groupDBID)
}

// GetRole returns a user's bot role in a group.
func (s *AdminService) GetRole(ctx context.Context, groupDBID, telegramID int64) postgres.BotRole {
	role, _ := s.adminRepo.GetRole(ctx, groupDBID, telegramID)
	return role
}
