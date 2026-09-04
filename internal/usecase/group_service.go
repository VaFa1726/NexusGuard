package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/nexusguard/bot/internal/domain"
	"github.com/nexusguard/bot/internal/repository/postgres"
)

// linkRegex matches URLs, telegram links, and link shorteners.
var linkRegex = regexp.MustCompile(`(?i)(https?://[^\s]+|t\.me/[^\s]+|telegram\.me/[^\s]+|bit\.ly/[^\s]+|tinyurl\.com/[^\s]+|www\.[^\s]+)`)

type xpEvent struct {
	groupID int64
	userID  int64
}

// GroupService contains the core business logic for group moderation.
type GroupService struct {
	groupRepo *postgres.GroupRepository
	userRepo  *postgres.UserRepository
	xpQueue   chan xpEvent
}

func NewGroupService(gr *postgres.GroupRepository, ur *postgres.UserRepository) *GroupService {
	s := &GroupService{
		groupRepo: gr,
		userRepo:  ur,
		xpQueue:   make(chan xpEvent, 500),
	}
	// Start background worker for XP updates to prevent DB saturation
	go s.startXPWorker()
	return s
}

func (s *GroupService) startXPWorker() {
	for ev := range s.xpQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		user := &domain.User{TelegramID: ev.userID}
		if err := s.userRepo.Upsert(ctx, user); err == nil {
			if member, err := s.userRepo.UpsertMember(ctx, ev.groupID, user.ID); err == nil {
				_ = s.userRepo.AddXP(ctx, member.ID, 1)
			}
		}
		cancel()
	}
}

// RegisterGroup upserts a group when the bot is added to it.
func (s *GroupService) RegisterGroup(ctx context.Context, telegramID int64, title, username string, ownerID int64) (*domain.Group, error) {
	g := &domain.Group{
		TelegramID: telegramID,
		Title:      title,
		Username:   username,
		OwnerID:    ownerID,
	}
	if err := s.groupRepo.Upsert(ctx, g); err != nil {
		return nil, fmt.Errorf("RegisterGroup: %w", err)
	}
	slog.Info("Group registered", "group_id", telegramID, "title", title)
	return g, nil
}

// GetGroup fetches a group's settings.
func (s *GroupService) GetGroup(ctx context.Context, telegramID int64) (*domain.Group, error) {
	return s.groupRepo.GetByTelegramID(ctx, telegramID)
}

// GetOwnerGroups returns all groups owned by a Telegram user.
func (s *GroupService) GetOwnerGroups(ctx context.Context, ownerID int64) ([]domain.Group, error) {
	return s.groupRepo.ListByOwner(ctx, ownerID)
}

// GetManagedGroups returns all groups where the user is an owner or admin.
func (s *GroupService) GetManagedGroups(ctx context.Context, telegramID int64) ([]domain.Group, error) {
	return s.groupRepo.ListManagedGroups(ctx, telegramID)
}

// SetGroupActive updates the is_active flag for a group.
func (s *GroupService) SetGroupActive(ctx context.Context, groupDBID int64, active bool) error {
	return s.groupRepo.SetGroupActive(ctx, groupDBID, active)
}

// ShouldFilterMessage checks if a message violates the group rules.
// Returns true if the message should be deleted.
func (s *GroupService) ShouldFilterMessage(ctx context.Context, group *domain.Group, text string) (bool, string) {
	if group.FilterLinks && linkRegex.MatchString(text) {
		return true, "Unauthorized link"
	}
	return false, ""
}

// WarnUser adds a warning to a user and returns the new count and action taken.
func (s *GroupService) WarnUser(
	ctx context.Context,
	group *domain.Group,
	targetTelegramID int64,
	adminTelegramID int64,
	reason string,
) (warnCount int, action string, err error) {
	// Upsert both user records
	target := &domain.User{TelegramID: targetTelegramID}
	if err = s.userRepo.Upsert(ctx, target); err != nil {
		return 0, "", fmt.Errorf("WarnUser upsert target: %w", err)
	}
	admin := &domain.User{TelegramID: adminTelegramID}
	if err = s.userRepo.Upsert(ctx, admin); err != nil {
		return 0, "", fmt.Errorf("WarnUser upsert admin: %w", err)
	}

	member, err := s.userRepo.UpsertMember(ctx, group.ID, target.ID)
	if err != nil {
		return 0, "", fmt.Errorf("WarnUser upsert member: %w", err)
	}

	warnCount, err = s.userRepo.IncrementWarn(ctx, member.ID, reason, group.ID, target.ID, admin.ID)
	if err != nil {
		return 0, "", fmt.Errorf("WarnUser increment: %w", err)
	}

	// Determine consequence
	switch {
	case warnCount >= group.MaxWarns*2: // double threshold → ban
		action = "ban"
	case warnCount >= group.MaxWarns: // hit threshold → mute
		action = "mute"
	default:
		action = "warn"
	}

	return warnCount, action, nil
}

// RegisterUser upserts a user into the DB.
func (s *GroupService) RegisterUser(ctx context.Context, u *domain.User) error {
	return s.userRepo.Upsert(ctx, u)
}

// AddXPForMessage queues an XP increment event non-blockingly.
func (s *GroupService) AddXPForMessage(ctx context.Context, group *domain.Group, telegramUserID int64) {
	select {
	case s.xpQueue <- xpEvent{groupID: group.ID, userID: telegramUserID}:
	default:
		// Drop XP increment if queue is saturated under high traffic to keep bot fast
	}
}

// UpdateSettings saves new group settings.
func (s *GroupService) UpdateSettings(ctx context.Context, group *domain.Group) error {
	return s.groupRepo.UpdateSettings(ctx, group)
}

// ─── Member Management ────────────────────────────────────────────────────────

// ListBannedMembers returns all banned members in a group.
func (s *GroupService) ListBannedMembers(ctx context.Context, groupDBID int64) ([]postgres.MemberInfo, error) {
	return s.userRepo.ListBannedMembers(ctx, groupDBID)
}

// ListWarnedMembers returns members who have at least one warning.
func (s *GroupService) ListWarnedMembers(ctx context.Context, groupDBID int64) ([]postgres.MemberInfo, error) {
	return s.userRepo.ListWarnedMembers(ctx, groupDBID)
}

// ListMutedMembers returns all currently muted members in a group.
func (s *GroupService) ListMutedMembers(ctx context.Context, groupDBID int64) ([]postgres.MemberInfo, error) {
	return s.userRepo.ListMutedMembers(ctx, groupDBID)
}

// ListAllMembers returns top N tracked members in a group sorted by XP.
func (s *GroupService) ListAllMembers(ctx context.Context, groupDBID int64, limit int) ([]postgres.MemberInfo, error) {
	return s.userRepo.ListAllMembers(ctx, groupDBID, limit)
}

// UnbanUser removes the ban flag in DB.
func (s *GroupService) UnbanUser(ctx context.Context, groupDBID, targetTelegramID int64) error {
	return s.userRepo.UnbanUser(ctx, groupDBID, targetTelegramID)
}

// UnmuteUser removes the mute flag in DB.
func (s *GroupService) UnmuteUser(ctx context.Context, groupDBID, targetTelegramID int64) error {
	return s.userRepo.UnmuteUser(ctx, groupDBID, targetTelegramID)
}

// ResetWarns clears all warnings for a user in a group.
func (s *GroupService) ResetWarns(ctx context.Context, groupDBID, targetTelegramID int64) error {
	return s.userRepo.ResetWarns(ctx, groupDBID, targetTelegramID)
}

// SetMuteStatus syncs the is_muted DB flag after a Telegram restriction action.
func (s *GroupService) SetMuteStatus(ctx context.Context, groupDBID, targetTelegramID int64, muted bool, muteUntil *time.Time) error {
	return s.userRepo.SetMuteStatus(ctx, groupDBID, targetTelegramID, muted, muteUntil)
}

// SetBanStatus syncs the is_banned DB flag after a Telegram ban action.
func (s *GroupService) SetBanStatus(ctx context.Context, groupDBID, targetTelegramID int64, banned bool) error {
	return s.userRepo.SetBanStatus(ctx, groupDBID, targetTelegramID, banned)
}

// SoftDeleteMember marks a user's membership as deleted when they leave/are removed.
func (s *GroupService) SoftDeleteMember(ctx context.Context, groupDBID, telegramID int64) error {
	return s.userRepo.SoftDeleteMember(ctx, groupDBID, telegramID)
}
