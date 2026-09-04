package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nexusguard/bot/internal/repository/postgres"
	tele "gopkg.in/telebot.v3"
)

// ─── Button definitions for member management ─────────────────────────────────
var (
	btnUnban      = tele.Btn{Unique: "btn_unban"}
	btnUnmute     = tele.Btn{Unique: "btn_unmute"}
	btnResetWarns = tele.Btn{Unique: "btn_reset_warns"}
)

// RegisterMemberHandlers attaches member management handlers to the bot.
func (h *Handler) RegisterMemberHandlers(b *tele.Bot) {
	b.Handle("/banned",  h.onBanned)
	b.Handle("/warned",  h.onWarned)
	b.Handle("/muted",   h.onMuted)
	b.Handle("/members", h.onMembers)
	b.Handle("/unmute",  h.onUnmute) // reply to muted user → immediate unmute
	b.Handle("/unban",   h.onUnban)  // reply to banned user → immediate unban

	b.Handle(&btnUnban,      h.onCallbackUnban)
	b.Handle(&btnUnmute,     h.onCallbackUnmute)
	b.Handle(&btnResetWarns, h.onCallbackResetWarns)
}

// ─── /banned — list banned users ──────────────────────────────────────────────
func (h *Handler) onBanned(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}
	if !h.requireGroupRole(c, group.ID, postgres.RoleModerator) {
		return nil
	}

	members, err := h.svc.ListBannedMembers(ctx, group.ID)
	if err != nil || len(members) == 0 {
		msg, _ := c.Bot().Send(c.Chat(), "✅ No banned users found.", tele.ModeMarkdown)
		if msg != nil {
			autoDeleteAfter(c.Bot(), msg, 20*time.Second)
		}
		return nil
	}

	text, menu := buildBannedList(members, group.TelegramID)
	msg, err := c.Bot().Send(c.Chat(), text, menu, tele.ModeMarkdown)
	if err == nil {
		autoDeleteAfter(c.Bot(), msg, 30*time.Second)
	}
	return nil
}

func buildBannedList(members []postgres.MemberInfo, groupTelegramID int64, extraRows ...tele.Row) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🚫 *Banned Users* (%d users)\n\n", len(members)))

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for i, m := range members {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("_...and %d more users_\n", len(members)-20))
			break
		}
		name := memberDisplayName(m)
		sb.WriteString(fmt.Sprintf("%d. 🚫 %s\n", i+1, name))

		rows = append(rows, menu.Row(
			menu.Data(
				fmt.Sprintf("🔓 Unban %s", truncate(name, 15)),
				btnUnban.Unique,
				fmt.Sprintf("%d:%d", m.TelegramID, groupTelegramID),
			),
		))
	}
	rows = append(rows, extraRows...)
	sb.WriteString("\n_This message auto-deletes in 30s_ 🧹")
	menu.Inline(rows...)
	return sb.String(), menu
}

// ─── /warned — list warned users ──────────────────────────────────────────────
func (h *Handler) onWarned(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}
	if !h.requireGroupRole(c, group.ID, postgres.RoleModerator) {
		return nil
	}

	members, err := h.svc.ListWarnedMembers(ctx, group.ID)
	if err != nil || len(members) == 0 {
		msg, _ := c.Bot().Send(c.Chat(), "✅ No users with warnings found.", tele.ModeMarkdown)
		if msg != nil {
			autoDeleteAfter(c.Bot(), msg, 20*time.Second)
		}
		return nil
	}

	text, menu := buildWarnedList(members, group.TelegramID, group.MaxWarns)
	msg, err := c.Bot().Send(c.Chat(), text, menu, tele.ModeMarkdown)
	if err == nil {
		autoDeleteAfter(c.Bot(), msg, 30*time.Second)
	}
	return nil
}

func buildWarnedList(members []postgres.MemberInfo, groupTelegramID int64, maxWarns int, extraRows ...tele.Row) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️ *Warned Users* (Max warnings: %d)\n\n", maxWarns))

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for i, m := range members {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("_...and %d more users_\n", len(members)-20))
			break
		}
		name := memberDisplayName(m)
		warnBar := buildWarnBar(m.WarnCount, maxWarns)
		status := ""
		if m.IsMuted {
			status = " 🔇"
		}
		if m.IsBanned {
			status = " 🚫"
		}
		sb.WriteString(fmt.Sprintf("%d. %s %s%s `[%d/%d]`\n", i+1, warnBar, name, status, m.WarnCount, maxWarns))

		rows = append(rows, menu.Row(
			menu.Data(
				fmt.Sprintf("🗑️ Clear %s", truncate(name, 12)),
				btnResetWarns.Unique,
				fmt.Sprintf("%d:%d", m.TelegramID, groupTelegramID),
			),
		))
	}
	rows = append(rows, extraRows...)
	sb.WriteString("\n_This message auto-deletes in 30s_ 🧹")
	menu.Inline(rows...)
	return sb.String(), menu
}

func buildWarnBar(count, max int) string {
	if max <= 0 {
		max = 3
	}
	filled := count * 5 / max
	if filled > 5 {
		filled = 5
	}
	bar := strings.Repeat("🟥", filled) + strings.Repeat("⬜", 5-filled)
	return bar
}

// ─── /muted — list muted users ────────────────────────────────────────────────
func (h *Handler) onMuted(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}
	if !h.requireGroupRole(c, group.ID, postgres.RoleModerator) {
		return nil
	}

	members, err := h.svc.ListMutedMembers(ctx, group.ID)
	if err != nil || len(members) == 0 {
		msg, _ := c.Bot().Send(c.Chat(), "✅ No muted users found.", tele.ModeMarkdown)
		if msg != nil {
			autoDeleteAfter(c.Bot(), msg, 20*time.Second)
		}
		return nil
	}

	text, menu := buildMutedList(members, group.TelegramID)
	msg, err := c.Bot().Send(c.Chat(), text, menu, tele.ModeMarkdown)
	if err == nil {
		autoDeleteAfter(c.Bot(), msg, 30*time.Second)
	}
	return nil
}

func buildMutedList(members []postgres.MemberInfo, groupTelegramID int64, extraRows ...tele.Row) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔇 *Muted Users* (%d users)\n\n", len(members)))

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	for i, m := range members {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("_...and %d more users_\n", len(members)-20))
			break
		}
		name := memberDisplayName(m)
		sb.WriteString(fmt.Sprintf("%d. 🔇 %s\n", i+1, name))

		rows = append(rows, menu.Row(
			menu.Data(
				fmt.Sprintf("🔊 Unmute %s", truncate(name, 12)),
				btnUnmute.Unique,
				fmt.Sprintf("%d:%d", m.TelegramID, groupTelegramID),
			),
		))
	}
	rows = append(rows, extraRows...)
	sb.WriteString("\n_This message auto-deletes in 30s_ 🧹")
	menu.Inline(rows...)
	return sb.String(), menu
}

// ─── /members — list all tracked members ──────────────────────────────────────
func (h *Handler) onMembers(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}
	if !h.requireGroupRole(c, group.ID, postgres.RoleModerator) {
		return nil
	}

	members, err := h.svc.ListAllMembers(ctx, group.ID, 30)
	if err != nil || len(members) == 0 {
		msg, _ := c.Bot().Send(c.Chat(), "📋 No members recorded in the database yet.\n_Members are registered once they send a message._", tele.ModeMarkdown)
		if msg != nil {
			autoDeleteAfter(c.Bot(), msg, 20*time.Second)
		}
		return nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👥 *Tracked Members* (%d users, ranked by XP)\n\n", len(members)))

	for i, m := range members {
		name := memberDisplayName(m)
		flags := ""
		if m.IsMuted {
			flags += "🔇"
		}
		if m.IsBanned {
			flags += "🚫"
		}
		if m.WarnCount > 0 {
			flags += fmt.Sprintf("⚠️%d", m.WarnCount)
		}
		if flags != "" {
			flags = " " + flags
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, name, flags))
	}
	sb.WriteString("\n_This message auto-deletes in 30s_ 🧹")

	msg, err := c.Bot().Send(c.Chat(), sb.String(), tele.ModeMarkdown)
	if err == nil {
		autoDeleteAfter(c.Bot(), msg, 30*time.Second)
	}
	return nil
}

// ─── /unmute — direct unmute via reply ────────────────────────────────────────
func (h *Handler) onUnmute(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}
	if !h.requireGroupRole(c, group.ID, postgres.RoleAdmin) {
		return nil
	}

	replied := c.Message().ReplyTo
	if replied == nil || replied.Sender == nil {
		msg, _ := c.Bot().Send(c.Chat(), "⚠️ Please reply to the muted user's message.", tele.ModeMarkdown)
		if msg != nil {
			autoDeleteAfter(c.Bot(), msg, 10*time.Second)
		}
		return nil
	}

	target := replied.Sender
	targetName := target.FirstName
	if target.Username != "" {
		targetName = "@" + target.Username
	}

	// Lift Telegram restriction
	_ = c.Bot().Restrict(c.Chat(), &tele.ChatMember{
		User:   target,
		Rights: tele.NoRestrictions(),
	})
	// Clear DB flags
	_ = h.svc.UnmuteUser(ctx, group.ID, target.ID)
	_ = h.svc.SetMuteStatus(ctx, group.ID, target.ID, false, nil)

	msg, _ := c.Bot().Send(c.Chat(),
		fmt.Sprintf("🔊 *%s* has been unmuted.", targetName), tele.ModeMarkdown)
	if msg != nil {
		autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	}
	return nil
}

// ─── /unban — direct unban via reply ──────────────────────────────────────────
func (h *Handler) onUnban(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}
	if !h.requireGroupRole(c, group.ID, postgres.RoleAdmin) {
		return nil
	}

	replied := c.Message().ReplyTo
	if replied == nil || replied.Sender == nil {
		msg, _ := c.Bot().Send(c.Chat(), "⚠️ Please reply to the banned user's message.", tele.ModeMarkdown)
		if msg != nil {
			autoDeleteAfter(c.Bot(), msg, 10*time.Second)
		}
		return nil
	}

	target := replied.Sender
	targetName := target.FirstName
	if target.Username != "" {
		targetName = "@" + target.Username
	}

	// Unban in Telegram
	chat := &tele.Chat{ID: c.Chat().ID}
	_ = c.Bot().Unban(chat, target)
	// Clear DB flags
	_ = h.svc.UnbanUser(ctx, group.ID, target.ID)
	_ = h.svc.SetBanStatus(ctx, group.ID, target.ID, false)

	msg, _ := c.Bot().Send(c.Chat(),
		fmt.Sprintf("🔓 *%s* has been unbanned.", targetName), tele.ModeMarkdown)
	if msg != nil {
		autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	}
	return nil
}

// ─── Callbacks ────────────────────────────────────────────────────────────────

// onCallbackUnban handles the "Unban" button press.
func (h *Handler) onCallbackUnban(c tele.Context) error {
	targetID, groupTelegramID, ok := parseActionCallbackData(c.Data())
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Invalid data"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	role := h.checkBotRole(ctx, group.ID, c.Sender().ID)
	if group.OwnerID != c.Sender().ID && !postgres.HasMinRole(role, postgres.RoleModerator) {
		return c.Respond(&tele.CallbackResponse{Text: "🚫 Permission denied"})
	}

	targetUser := &tele.User{ID: targetID}
	chat := &tele.Chat{ID: groupTelegramID}
	_ = c.Bot().Unban(chat, targetUser)
	_ = h.svc.UnbanUser(ctx, group.ID, targetID)
	_ = h.svc.SetBanStatus(ctx, group.ID, targetID, false)

	return c.Respond(&tele.CallbackResponse{Text: "✅ User has been unbanned"})
}

// onCallbackUnmute handles the "Unmute" button press.
func (h *Handler) onCallbackUnmute(c tele.Context) error {
	targetID, groupTelegramID, ok := parseActionCallbackData(c.Data())
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Invalid data"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	role := h.checkBotRole(ctx, group.ID, c.Sender().ID)
	if group.OwnerID != c.Sender().ID && !postgres.HasMinRole(role, postgres.RoleModerator) {
		return c.Respond(&tele.CallbackResponse{Text: "🚫 Permission denied"})
	}

	targetUser := &tele.User{ID: targetID}
	chat := &tele.Chat{ID: groupTelegramID}
	_ = c.Bot().Restrict(chat, &tele.ChatMember{
		User:   targetUser,
		Rights: tele.NoRestrictions(),
	})
	_ = h.svc.UnmuteUser(ctx, group.ID, targetID)
	_ = h.svc.SetMuteStatus(ctx, group.ID, targetID, false, nil)

	return c.Respond(&tele.CallbackResponse{Text: "✅ User has been unmuted"})
}

// onCallbackResetWarns handles the "Clear warnings" button press.
func (h *Handler) onCallbackResetWarns(c tele.Context) error {
	targetID, groupTelegramID, ok := parseActionCallbackData(c.Data())
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Invalid data"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	role := h.checkBotRole(ctx, group.ID, c.Sender().ID)
	if group.OwnerID != c.Sender().ID && !postgres.HasMinRole(role, postgres.RoleAdmin) {
		return c.Respond(&tele.CallbackResponse{Text: "🚫 Only an admin or owner can reset warnings"})
	}

	if err := h.svc.ResetWarns(ctx, group.ID, targetID); err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Failed to reset warnings"})
	}

	return c.Respond(&tele.CallbackResponse{Text: "✅ Warnings reset to 0"})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func parseActionCallbackData(data string) (targetID, groupTelegramID int64, ok bool) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	t, err1 := strconv.ParseInt(parts[0], 10, 64)
	g, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return t, g, true
}

func memberDisplayName(m postgres.MemberInfo) string {
	if m.Username != "" {
		return "@" + m.Username
	}
	if m.FirstName != "" {
		return m.FirstName
	}
	return fmt.Sprintf("user_%d", m.TelegramID)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
