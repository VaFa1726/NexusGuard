package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BotRole defines permission levels inside NexusGuard.
// Independent from Telegram's own admin system.
type BotRole string

const (
	RoleOwner     BotRole = "owner"     // Full access — set automatically when bot is added
	RoleAdmin     BotRole = "admin"     // Can warn, mute, ban, change settings
	RoleModerator BotRole = "moderator" // Can warn only
)

// BotAdmin represents a user who has been granted a bot role in a group.
type BotAdmin struct {
	ID          int64     `db:"id"`
	GroupID     int64     `db:"group_id"`      // internal DB group id
	TelegramID  int64     `db:"telegram_id"`   // user's telegram id
	Username    string    `db:"username"`
	Role        BotRole   `db:"role"`
	GrantedBy   int64     `db:"granted_by"`    // telegram id of granter
	CreatedAt   time.Time `db:"created_at"`
}

type AdminRepository struct {
	pool *pgxpool.Pool
}

func NewAdminRepository(pool *pgxpool.Pool) *AdminRepository {
	return &AdminRepository{pool: pool}
}

// SetRole grants or updates a role for a user in a group.
func (r *AdminRepository) SetRole(ctx context.Context, groupID, telegramID, grantedBy int64, username string, role BotRole) error {
	query := `
		INSERT INTO group_bot_admins (group_id, telegram_id, username, role, granted_by, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (group_id, telegram_id) DO UPDATE
		SET role = EXCLUDED.role,
		    username = EXCLUDED.username,
		    granted_by = EXCLUDED.granted_by,
		    created_at = NOW()`
	_, err := r.pool.Exec(ctx, query, groupID, telegramID, username, role, grantedBy)
	return err
}

// RemoveRole revokes a user's bot role in a group.
func (r *AdminRepository) RemoveRole(ctx context.Context, groupID, telegramID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM group_bot_admins WHERE group_id = $1 AND telegram_id = $2 AND role != 'owner'`,
		groupID, telegramID,
	)
	return err
}

// GetRole returns the bot role for a user in a group, or empty string if none.
func (r *AdminRepository) GetRole(ctx context.Context, groupDBID, telegramID int64) (BotRole, error) {
	var role BotRole
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM group_bot_admins WHERE group_id = $1 AND telegram_id = $2`,
		groupDBID, telegramID,
	).Scan(&role)
	if err != nil {
		return "", nil // no role = regular member
	}
	return role, nil
}

// ListAdmins returns all bot admins/moderators for a group.
func (r *AdminRepository) ListAdmins(ctx context.Context, groupDBID int64) ([]BotAdmin, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, group_id, telegram_id, username, role, granted_by, created_at
		 FROM group_bot_admins WHERE group_id = $1 ORDER BY role, created_at`,
		groupDBID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var admins []BotAdmin
	for rows.Next() {
		var a BotAdmin
		if err := rows.Scan(&a.ID, &a.GroupID, &a.TelegramID, &a.Username,
			&a.Role, &a.GrantedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		admins = append(admins, a)
	}
	return admins, rows.Err()
}

// HasMinRole checks if a user has at least the given role level.
func HasMinRole(userRole BotRole, minRole BotRole) bool {
	levels := map[BotRole]int{
		RoleModerator: 1,
		RoleAdmin:     2,
		RoleOwner:     3,
	}
	return levels[userRole] >= levels[minRole]
}

// HasAnyRole returns true if the user holds any bot role in any group.
// Used to gate access to the bot's private chat interface.
func (r *AdminRepository) HasAnyRole(ctx context.Context, telegramID int64) bool {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM group_bot_admins WHERE telegram_id = $1)`,
		telegramID,
	).Scan(&exists)
	return err == nil && exists
}

// GetRoleInAnyGroup returns the highest role a user holds across all groups.
// Returns empty string if the user has no roles.
func (r *AdminRepository) GetRoleInAnyGroup(ctx context.Context, telegramID int64) BotRole {
	var role BotRole
	// Order by role priority: owner > admin > moderator
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM group_bot_admins
		 WHERE telegram_id = $1
		 ORDER BY CASE role
		   WHEN 'owner'     THEN 3
		   WHEN 'admin'     THEN 2
		   WHEN 'moderator' THEN 1
		   ELSE 0
		 END DESC
		 LIMIT 1`,
		telegramID,
	).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}
