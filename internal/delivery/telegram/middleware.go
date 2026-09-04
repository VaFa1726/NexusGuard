package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nexusguard/bot/internal/repository/postgres"
	tele "gopkg.in/telebot.v3"
)

// ─── Auto-delete helpers ──────────────────────────────────────────────────────

// autoDeleteAfter deletes a message after the given duration.
// Runs in a goroutine — non-blocking.
func autoDeleteAfter(bot *tele.Bot, msg *tele.Message, delay time.Duration) {
	go func() {
		time.Sleep(delay)
		if err := bot.Delete(msg); err != nil {
			slog.Debug("auto-delete failed (msg may already be gone)", "error", err)
		}
	}()
}

// deleteCommandMsg immediately deletes the admin's command message so
// other members never see it.
func deleteCommandMsg(c tele.Context) {
	if c.Message() == nil {
		return
	}
	if err := c.Bot().Delete(c.Message()); err != nil {
		slog.Debug("failed to delete command msg", "error", err)
	}
}

// sendEphemeral sends a message to the chat and auto-deletes it after 20s.
// Also deletes the triggering command message immediately.
func sendEphemeral(c tele.Context, text string, opts ...interface{}) error {
	deleteCommandMsg(c)
	opts = append(opts, tele.ModeMarkdown)
	msg, err := c.Bot().Send(c.Chat(), text, opts...)
	if err != nil {
		return err
	}
	autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	return nil
}

// sendEphemeralWithMenu sends a message with inline keyboard and auto-deletes after 20s.
func sendEphemeralWithMenu(c tele.Context, text string, menu *tele.ReplyMarkup) error {
	deleteCommandMsg(c)
	msg, err := c.Bot().Send(c.Chat(), text, menu, tele.ModeMarkdown)
	if err != nil {
		return err
	}
	autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	return nil
}

// ─── Private Chat Authentication Gate ────────────────────────────────────────

// requirePrivateAuth checks if the sender is authorized to use the bot in private chat.
// Only users who are the Owner of at least one group may interact with the PV dashboard.
// Admins and Moderators are restricted to in-group commands only.
// If unauthorized: silently does nothing and returns false.
func (h *Handler) requirePrivateAuth(c tele.Context) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	authorized := h.adminRepo.IsOwnerOfAnyGroup(ctx, c.Sender().ID)
	if !authorized {
		slog.Debug("Unauthorized private chat attempt — user is not owner of any group",
			"user_id", c.Sender().ID,
			"username", c.Sender().Username,
		)
		// Silent — no response to unauthorized users
		return false
	}
	return true
}

// ─── Group Permission checker ─────────────────────────────────────────────────

// checkBotRole returns the user's NexusGuard role in the group.
func (h *Handler) checkBotRole(ctx context.Context, groupDBID, telegramID int64) postgres.BotRole {
	role, err := h.adminRepo.GetRole(ctx, groupDBID, telegramID)
	if err != nil || role == "" {
		return "" // no role
	}
	return role
}

// requireGroupRole checks if the sender has at least minRole in the group.
// Silently deletes the command and returns false if unauthorized.
func (h *Handler) requireGroupRole(c tele.Context, groupDBID int64, minRole postgres.BotRole) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Always delete the command message immediately
	deleteCommandMsg(c)

	role := h.checkBotRole(ctx, groupDBID, c.Sender().ID)
	if !postgres.HasMinRole(role, minRole) {
		// Complete silence for unauthorized users in groups
		return false
	}
	return true
}

// requireOwnerOnly checks that the sender is the group owner.
// Silently deletes command and returns false if not owner.
func (h *Handler) requireOwnerOnly(c tele.Context, groupDBID int64) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	deleteCommandMsg(c)

	role := h.checkBotRole(ctx, groupDBID, c.Sender().ID)
	return role == postgres.RoleOwner
}

// ─── Callback Security ────────────────────────────────────────────────────────

// buildSecureCallbackData encodes admin Telegram ID into callback data
// so only the admin who triggered the menu can interact with it.
// Format: "uniqueKey:adminTelegramID:groupTelegramID"
func buildSecureCallbackData(unique string, adminID, groupTelegramID int64) string {
	return fmt.Sprintf("%s:%d:%d", unique, adminID, groupTelegramID)
}

// parseSecureCallbackData parses the callback data and verifies the caller is the original admin.
// Returns (groupTelegramID, authorized).
func parseSecureCallbackData(c tele.Context) (groupTelegramID int64, authorized bool) {
	data := c.Data()
	var uniqueKey string
	var adminID int64
	_, err := fmt.Sscanf(data, "%s:%d:%d", &uniqueKey, &adminID, &groupTelegramID)
	if err != nil {
		return 0, false
	}
	return groupTelegramID, c.Sender().ID == adminID
}

// answerUnauthorizedCallback silently answers a callback query without alerting the user.
// Used when an unauthorized user clicks a button — they get no visible feedback.
func answerUnauthorizedCallback(c tele.Context) error {
	return c.Respond(&tele.CallbackResponse{Text: ""})
}
