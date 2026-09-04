package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nexusguard/bot/internal/domain"
)

type GroupRepository struct {
	pool *pgxpool.Pool
}

func NewGroupRepository(pool *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{pool: pool}
}

// Upsert creates or updates a group record.
func (r *GroupRepository) Upsert(ctx context.Context, g *domain.Group) error {
	query := `
		INSERT INTO groups (telegram_id, title, username, owner_id, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (telegram_id) DO UPDATE
		SET title = EXCLUDED.title,
		    username = EXCLUDED.username,
		    updated_at = NOW()
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		g.TelegramID, g.Title, g.Username, g.OwnerID,
	).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
}

// GetByTelegramID fetches a group by its Telegram chat ID.
func (r *GroupRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.Group, error) {
	g := &domain.Group{}
	query := `SELECT id, telegram_id, title, username, owner_id, is_active,
	           filter_links, filter_profanity, welcome_enabled, welcome_message,
	           max_warns, mute_duration, created_at, updated_at
	          FROM groups WHERE telegram_id = $1`
	err := r.pool.QueryRow(ctx, query, telegramID).Scan(
		&g.ID, &g.TelegramID, &g.Title, &g.Username, &g.OwnerID, &g.IsActive,
		&g.FilterLinks, &g.FilterProfanity, &g.WelcomeEnabled, &g.WelcomeMessage,
		&g.MaxWarns, &g.MuteDuration, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// UpdateSettings saves changed settings for a group.
func (r *GroupRepository) UpdateSettings(ctx context.Context, g *domain.Group) error {
	query := `UPDATE groups SET
		is_active = $1, filter_links = $2, filter_profanity = $3,
		welcome_enabled = $4, welcome_message = $5, max_warns = $6,
		mute_duration = $7, updated_at = $8
		WHERE telegram_id = $9`
	_, err := r.pool.Exec(ctx, query,
		g.IsActive, g.FilterLinks, g.FilterProfanity,
		g.WelcomeEnabled, g.WelcomeMessage, g.MaxWarns,
		g.MuteDuration, time.Now(), g.TelegramID,
	)
	return err
}

// ListByOwner returns all groups owned by a user.
func (r *GroupRepository) ListByOwner(ctx context.Context, ownerTelegramID int64) ([]domain.Group, error) {
	query := `SELECT id, telegram_id, title, username, owner_id, is_active,
	           filter_links, filter_profanity, welcome_enabled, welcome_message,
	           max_warns, mute_duration, created_at, updated_at
	          FROM groups WHERE owner_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, ownerTelegramID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []domain.Group
	for rows.Next() {
		var g domain.Group
		if err := rows.Scan(
			&g.ID, &g.TelegramID, &g.Title, &g.Username, &g.OwnerID, &g.IsActive,
			&g.FilterLinks, &g.FilterProfanity, &g.WelcomeEnabled, &g.WelcomeMessage,
			&g.MaxWarns, &g.MuteDuration, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// ListManagedGroups returns all groups where the user is either the owner or has a bot admin role.
// Limited to 100 groups for performance.
func (r *GroupRepository) ListManagedGroups(ctx context.Context, telegramID int64) ([]domain.Group, error) {
	query := `SELECT DISTINCT g.id, g.telegram_id, g.title, g.username, g.owner_id, g.is_active,
	           g.filter_links, g.filter_profanity, g.welcome_enabled, g.welcome_message,
	           g.max_warns, g.mute_duration, g.created_at, g.updated_at
	          FROM groups g
	          LEFT JOIN group_bot_admins gba ON gba.group_id = g.id
	          WHERE g.owner_id = $1 OR gba.telegram_id = $1
	          ORDER BY g.created_at DESC
	          LIMIT 100`
	rows, err := r.pool.Query(ctx, query, telegramID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []domain.Group
	for rows.Next() {
		var g domain.Group
		if err := rows.Scan(
			&g.ID, &g.TelegramID, &g.Title, &g.Username, &g.OwnerID, &g.IsActive,
			&g.FilterLinks, &g.FilterProfanity, &g.WelcomeEnabled, &g.WelcomeMessage,
			&g.MaxWarns, &g.MuteDuration, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// SetGroupActive updates the is_active flag for a group.
func (r *GroupRepository) SetGroupActive(ctx context.Context, groupDBID int64, active bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE groups SET is_active = $1, updated_at = NOW() WHERE id = $2`,
		active, groupDBID,
	)
	return err
}
