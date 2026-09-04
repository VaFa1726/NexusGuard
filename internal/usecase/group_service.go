package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/nexusguard/bot/internal/domain"
	"github.com/nexusguard/bot/internal/repository/postgres"
)

// linkRegex matches URLs, telegram links, and link shorteners.
var linkRegex = regexp.MustCompile(`(?i)(https?://[^\s]+|t\.me/[^\s]+|telegram\.me/[^\s]+|bit\.ly/[^\s]+|tinyurl\.com/[^\s]+|www\.[^\s]+)`)

// profanityRegex matches common profanity and offensive words (basic filter).
// Note: This is a basic implementation. For production, consider using a comprehensive profanity library.
var profanityRegex = regexp.MustCompile(`(?i)(fuck|shit|bitch|asshole|damn|crap|bastard|dick|pussy|cock|کیر|کس|جنده|گاو|خر|احمق|کونی)`)

type xpEvent struct {
	groupID int64
	userID  int64
}

type xpBatch struct {
	events []xpEvent
	mu     sync.Mutex
}

// GroupService contains the core business logic for group moderation.
type GroupService struct {
	groupRepo *postgres.GroupRepository
	userRepo  *postgres.UserRepository
	xpQueue   chan xpEvent
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func NewGroupService(gr *postgres.GroupRepository, ur *postgres.UserRepository) *GroupService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &GroupService{
		groupRepo: gr,
		userRepo:  ur,
		xpQueue:   make(chan xpEvent, 5000), // Increased from 500
		ctx:       ctx,
		cancel:    cancel,
	}
	// Start 10 background workers for XP updates to prevent DB saturation
	for i := 0; i < 10; i++ {
		s.wg.Add(1)
		go s.xpWorkerPool(i)
	}
	slog.Info("XP worker pool started", "workers", 10, "queue_size", 5000)
	return s
}

// xpWorkerPool processes XP events in batches for better performance
func (s *GroupService) xpWorkerPool(workerID int) {
	defer s.wg.Done()
	
	batch := make(map[string]*xpEvent) // key: "groupID:userID"
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		// Process batch
		for _, ev := range batch {
			user := &domain.User{TelegramID: ev.userID}
			if err := s.userRepo.Upsert(ctx, user); err == nil {
				if member, err := s.userRepo.UpsertMember(ctx, ev.groupID, user.ID); err == nil {
					_ = s.userRepo.AddXP(ctx, member.ID, 1)
				}
			}
		}
		
		slog.Debug("XP batch processed", "worker", workerID, "count", len(batch))
		batch = make(map[string]*xpEvent)
	}
	
	for {
		select {
		case ev := <-s.xpQueue:
			key := fmt.Sprintf("%d:%d", ev.groupID, ev.userID)
			batch[key] = &ev
			
			// Flush if batch is large enough
			if len(batch) >= 100 {
				flushBatch()
			}
			
		case <-ticker.C:
			flushBatch()
			
		case <-s.ctx.Done():
			flushBatch() // Final flush
			slog.Info("XP worker stopped", "worker", workerID)
			return
		}
	}
}

// Shutdown gracefully stops all XP workers
func (s *GroupService) Shutdown() {
	slog.Info("Shutting down XP workers...")
	s.cancel()
	s.wg.Wait()
	close(s.xpQueue)
	slog.Info("XP workers stopped gracefully")
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
	if group.FilterProfanity && profanityRegex.MatchString(text) {
		return true, "Inappropriate language"
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

// AddXPForMessage queues an XP increment event non-blockingly with metrics.
func (s *GroupService) AddXPForMessage(ctx context.Context, group *domain.Group, telegramUserID int64) {
	select {
	case s.xpQueue <- xpEvent{groupID: group.ID, userID: telegramUserID}:
		// Successfully queued
	default:
		// Queue full - log warning instead of silent drop
		slog.Warn("XP queue full - dropping event",
			"user_id", telegramUserID,
			"group_id", group.ID,
			"queue_size", len(s.xpQueue))
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
