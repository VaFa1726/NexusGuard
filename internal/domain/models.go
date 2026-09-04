package domain

import (
	"time"
)

// Group represents a Telegram group managed by the bot.
type Group struct {
	ID              int64     `db:"id"`
	TelegramID      int64     `db:"telegram_id"`
	Title           string    `db:"title"`
	Username        string    `db:"username"`
	OwnerID         int64     `db:"owner_id"`
	IsActive        bool      `db:"is_active"`
	FilterLinks     bool      `db:"filter_links"`
	FilterProfanity bool      `db:"filter_profanity"`
	WelcomeEnabled  bool      `db:"welcome_enabled"`
	WelcomeMessage  string    `db:"welcome_message"`
	MaxWarns        int       `db:"max_warns"`
	MuteDuration    int       `db:"mute_duration"` // minutes
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

// User represents a Telegram user tracked by the bot.
type User struct {
	ID         int64     `db:"id"`
	TelegramID int64     `db:"telegram_id"`
	Username   string    `db:"username"`
	FirstName  string    `db:"first_name"`
	LastName   string    `db:"last_name"`
	IsBot      bool      `db:"is_bot"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// GroupMember represents a user's membership and stats in a group.
type GroupMember struct {
	ID        int64      `db:"id"`
	GroupID   int64      `db:"group_id"`
	UserID    int64      `db:"user_id"`
	WarnCount int        `db:"warn_count"`
	XP        int        `db:"xp"`
	Level     int        `db:"level"`
	IsMuted   bool       `db:"is_muted"`
	IsBanned  bool       `db:"is_banned"`
	MuteUntil *time.Time `db:"mute_until"`
	JoinedAt  time.Time  `db:"joined_at"`
	UpdatedAt time.Time  `db:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at"` // soft delete
}

// WarnLog stores the history of warnings given to users.
type WarnLog struct {
	ID        int64     `db:"id"`
	GroupID   int64     `db:"group_id"`
	UserID    int64     `db:"user_id"`
	AdminID   int64     `db:"admin_id"`
	Reason    string    `db:"reason"`
	CreatedAt time.Time `db:"created_at"`
}

// ActionLog stores moderation actions.
type ActionLog struct {
	ID        int64      `db:"id"`
	GroupID   int64      `db:"group_id"`
	UserID    int64      `db:"user_id"`
	AdminID   int64      `db:"admin_id"`
	Action    string     `db:"action"` // "warn", "mute", "ban", "kick", "delete"
	Reason    string     `db:"reason"`
	ExpiresAt *time.Time `db:"expires_at"`
	CreatedAt time.Time  `db:"created_at"`
}
