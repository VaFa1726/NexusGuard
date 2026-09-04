package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nexusguard/bot/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Upsert creates or updates a user record. Returns the internal DB ID.
func (r *UserRepository) Upsert(ctx context.Context, u *domain.User) error {
	query := `
		INSERT INTO users (telegram_id, username, first_name, last_name, is_bot, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (telegram_id) DO UPDATE
		SET username   = EXCLUDED.username,
		    first_name = EXCLUDED.first_name,
		    last_name  = EXCLUDED.last_name,
		    updated_at = NOW()
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		u.TelegramID, u.Username, u.FirstName, u.LastName, u.IsBot,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

// GetByTelegramID fetches a user by their Telegram user ID.
func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	u := &domain.User{}
	query := `SELECT id, telegram_id, username, first_name, last_name, is_bot, created_at, updated_at
	          FROM users WHERE telegram_id = $1`
	err := r.pool.QueryRow(ctx, query, telegramID).Scan(
		&u.ID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName,
		&u.IsBot, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetMember returns a group membership record.
func (r *UserRepository) GetMember(ctx context.Context, groupID, userID int64) (*domain.GroupMember, error) {
	m := &domain.GroupMember{}
	query := `SELECT id, group_id, user_id, warn_count, xp, level, is_muted, is_banned, mute_until, joined_at, updated_at
	          FROM group_members
	          WHERE group_id = $1 AND user_id = $2 AND deleted_at IS NULL`
	err := r.pool.QueryRow(ctx, query, groupID, userID).Scan(
		&m.ID, &m.GroupID, &m.UserID, &m.WarnCount, &m.XP, &m.Level,
		&m.IsMuted, &m.IsBanned, &m.MuteUntil, &m.JoinedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// UpsertMember creates or ensures a group_member record exists.
func (r *UserRepository) UpsertMember(ctx context.Context, groupDBID, userDBID int64) (*domain.GroupMember, error) {
	query := `
		INSERT INTO group_members (group_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (group_id, user_id) DO UPDATE SET updated_at = NOW()
		RETURNING id, group_id, user_id, warn_count, xp, level, is_muted, is_banned, mute_until, joined_at, updated_at`
	m := &domain.GroupMember{}
	err := r.pool.QueryRow(ctx, query, groupDBID, userDBID).Scan(
		&m.ID, &m.GroupID, &m.UserID, &m.WarnCount, &m.XP, &m.Level,
		&m.IsMuted, &m.IsBanned, &m.MuteUntil, &m.JoinedAt, &m.UpdatedAt,
	)
	return m, err
}

// IncrementWarn adds a warn and returns new warn count.
func (r *UserRepository) IncrementWarn(ctx context.Context, memberID int64, reason string, groupID, userID, adminID int64) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var warnCount int
	err = tx.QueryRow(ctx,
		`UPDATE group_members SET warn_count = warn_count + 1, updated_at = NOW()
		 WHERE id = $1 RETURNING warn_count`, memberID,
	).Scan(&warnCount)
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO warn_logs (group_id, user_id, admin_id, reason) VALUES ($1, $2, $3, $4)`,
		groupID, userID, adminID, reason,
	)
	if err != nil {
		return 0, err
	}

	return warnCount, tx.Commit(ctx)
}

// AddXP increments a user's XP in a group.
func (r *UserRepository) AddXP(ctx context.Context, memberID int64, amount int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE group_members SET xp = xp + $1, updated_at = NOW() WHERE id = $2`,
		amount, memberID,
	)
	return err
}

// SetMuteStatus updates the is_muted flag and mute_until timestamp for a user in a group.
func (r *UserRepository) SetMuteStatus(ctx context.Context, groupDBID, targetTelegramID int64, muted bool, muteUntil *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE group_members gm SET is_muted = $3, mute_until = $4, updated_at = NOW()
		 FROM users u
		 WHERE gm.user_id = u.id AND gm.group_id = $1 AND u.telegram_id = $2`,
		groupDBID, targetTelegramID, muted, muteUntil,
	)
	return err
}

// SetBanStatus updates the is_banned flag for a user in a group.
func (r *UserRepository) SetBanStatus(ctx context.Context, groupDBID, targetTelegramID int64, banned bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE group_members gm SET is_banned = $3, updated_at = NOW()
		 FROM users u
		 WHERE gm.user_id = u.id AND gm.group_id = $1 AND u.telegram_id = $2`,
		groupDBID, targetTelegramID, banned,
	)
	return err
}

// ─── Member Management ────────────────────────────────────────────────────────

// MemberInfo holds user info alongside their group membership data.
type MemberInfo struct {
	TelegramID int64
	Username   string
	FirstName  string
	WarnCount  int
	IsMuted    bool
	IsBanned   bool
}

// ListBannedMembers returns all currently banned members in a group.
func (r *UserRepository) ListBannedMembers(ctx context.Context, groupDBID int64) ([]MemberInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.telegram_id, u.username, u.first_name, gm.warn_count, gm.is_muted, gm.is_banned
		 FROM group_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.group_id = $1 AND gm.is_banned = TRUE AND gm.deleted_at IS NULL
		 ORDER BY gm.updated_at DESC`,
		groupDBID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemberInfoRows(rows)
}

// ListWarnedMembers returns all members with at least one warning.
func (r *UserRepository) ListWarnedMembers(ctx context.Context, groupDBID int64) ([]MemberInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.telegram_id, u.username, u.first_name, gm.warn_count, gm.is_muted, gm.is_banned
		 FROM group_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.group_id = $1 AND gm.warn_count > 0 AND gm.deleted_at IS NULL
		 ORDER BY gm.warn_count DESC`,
		groupDBID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemberInfoRows(rows)
}

// ListMutedMembers returns all currently muted members in a group.
func (r *UserRepository) ListMutedMembers(ctx context.Context, groupDBID int64) ([]MemberInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.telegram_id, u.username, u.first_name, gm.warn_count, gm.is_muted, gm.is_banned
		 FROM group_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.group_id = $1 AND gm.is_muted = TRUE AND gm.deleted_at IS NULL
		 ORDER BY gm.updated_at DESC`,
		groupDBID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemberInfoRows(rows)
}

// ListAllMembers returns all known members of a group (tracked by the bot).
func (r *UserRepository) ListAllMembers(ctx context.Context, groupDBID int64, limit int) ([]MemberInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.telegram_id, u.username, u.first_name, gm.warn_count, gm.is_muted, gm.is_banned
		 FROM group_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.group_id = $1 AND gm.deleted_at IS NULL
		 ORDER BY gm.xp DESC, gm.joined_at ASC
		 LIMIT $2`,
		groupDBID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemberInfoRows(rows)
}

// ResetWarns sets warn_count to 0 for a specific user in a group.
func (r *UserRepository) ResetWarns(ctx context.Context, groupDBID, targetTelegramID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE group_members gm SET warn_count = 0, updated_at = NOW()
		 FROM users u
		 WHERE gm.user_id = u.id AND gm.group_id = $1 AND u.telegram_id = $2`,
		groupDBID, targetTelegramID,
	)
	return err
}

// UnmuteUser clears the mute flag for a specific user in a group.
func (r *UserRepository) UnmuteUser(ctx context.Context, groupDBID, targetTelegramID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE group_members gm SET is_muted = FALSE, mute_until = NULL, updated_at = NOW()
		 FROM users u
		 WHERE gm.user_id = u.id AND gm.group_id = $1 AND u.telegram_id = $2`,
		groupDBID, targetTelegramID,
	)
	return err
}

// UnbanUser clears the ban flag for a specific user in a group (DB-side only).
func (r *UserRepository) UnbanUser(ctx context.Context, groupDBID, targetTelegramID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE group_members gm SET is_banned = FALSE, updated_at = NOW()
		 FROM users u
		 WHERE gm.user_id = u.id AND gm.group_id = $1 AND u.telegram_id = $2`,
		groupDBID, targetTelegramID,
	)
	return err
}

func scanMemberInfoRows(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
}) ([]MemberInfo, error) {
	var members []MemberInfo
	for rows.Next() {
		var m MemberInfo
		if err := rows.Scan(&m.TelegramID, &m.Username, &m.FirstName, &m.WarnCount, &m.IsMuted, &m.IsBanned); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// SoftDeleteMember marks a user's group membership as deleted.
// Used when a user leaves or is removed from the group.
func (r *UserRepository) SoftDeleteMember(ctx context.Context, groupDBID, telegramID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE group_members gm SET deleted_at = NOW(), updated_at = NOW()
		 FROM users u
		 WHERE gm.user_id = u.id AND gm.group_id = $1 AND u.telegram_id = $2
		   AND gm.deleted_at IS NULL`,
		groupDBID, telegramID,
	)
	return err
}
