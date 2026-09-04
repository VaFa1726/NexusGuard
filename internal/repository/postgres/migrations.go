package postgres

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunMigrations creates all necessary tables if they do not exist.
// This is a simple migration approach suitable for Phase 1.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("Running database migrations...")

	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id          BIGSERIAL PRIMARY KEY,
			telegram_id BIGINT      NOT NULL UNIQUE,
			username    TEXT        NOT NULL DEFAULT '',
			first_name  TEXT        NOT NULL DEFAULT '',
			last_name   TEXT        NOT NULL DEFAULT '',
			is_bot      BOOLEAN     NOT NULL DEFAULT FALSE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id)`,

		`CREATE TABLE IF NOT EXISTS groups (
			id               BIGSERIAL PRIMARY KEY,
			telegram_id      BIGINT      NOT NULL UNIQUE,
			title            TEXT        NOT NULL DEFAULT '',
			username         TEXT        NOT NULL DEFAULT '',
			owner_id         BIGINT      NOT NULL,
			is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
			filter_links     BOOLEAN     NOT NULL DEFAULT TRUE,
			filter_profanity BOOLEAN     NOT NULL DEFAULT FALSE,
			welcome_enabled  BOOLEAN     NOT NULL DEFAULT TRUE,
			welcome_message  TEXT        NOT NULL DEFAULT '',
			max_warns        INT         NOT NULL DEFAULT 3,
			mute_duration    INT         NOT NULL DEFAULT 1440,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_groups_telegram_id ON groups(telegram_id)`,
		`CREATE INDEX IF NOT EXISTS idx_groups_owner_id ON groups(owner_id)`,

		`CREATE TABLE IF NOT EXISTS group_members (
			id          BIGSERIAL PRIMARY KEY,
			group_id    BIGINT      NOT NULL REFERENCES groups(id),
			user_id     BIGINT      NOT NULL REFERENCES users(id),
			warn_count  INT         NOT NULL DEFAULT 0,
			xp          INT         NOT NULL DEFAULT 0,
			level       INT         NOT NULL DEFAULT 1,
			is_muted    BOOLEAN     NOT NULL DEFAULT FALSE,
			is_banned   BOOLEAN     NOT NULL DEFAULT FALSE,
			mute_until  TIMESTAMPTZ,
			joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at  TIMESTAMPTZ,
			UNIQUE(group_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_members_group_id ON group_members(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_members_user_id  ON group_members(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_members_banned   ON group_members(group_id) WHERE is_banned = TRUE AND deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_members_muted    ON group_members(group_id) WHERE is_muted = TRUE AND deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_members_warned   ON group_members(group_id, warn_count DESC) WHERE warn_count > 0 AND deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_members_xp       ON group_members(group_id, xp DESC) WHERE deleted_at IS NULL`,

		`CREATE TABLE IF NOT EXISTS warn_logs (
			id         BIGSERIAL PRIMARY KEY,
			group_id   BIGINT      NOT NULL REFERENCES groups(id),
			user_id    BIGINT      NOT NULL REFERENCES users(id),
			admin_id   BIGINT      NOT NULL,
			reason     TEXT        NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_warn_logs_group_user ON warn_logs(group_id, user_id)`,

		`CREATE TABLE IF NOT EXISTS group_bot_admins (
			id          BIGSERIAL PRIMARY KEY,
			group_id    BIGINT      NOT NULL REFERENCES groups(id),
			telegram_id BIGINT      NOT NULL,
			username    TEXT        NOT NULL DEFAULT '',
			role        TEXT        NOT NULL DEFAULT 'moderator',
			granted_by  BIGINT      NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(group_id, telegram_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bot_admins_group ON group_bot_admins(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_bot_admins_tg_id ON group_bot_admins(telegram_id)`,
	}

	for _, q := range queries {
		if _, err := pool.Exec(ctx, q); err != nil {
			return err
		}
	}

	slog.Info("Database migrations completed successfully")
	return nil
}
