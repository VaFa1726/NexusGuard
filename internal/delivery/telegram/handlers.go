package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nexusguard/bot/internal/domain"
	"github.com/nexusguard/bot/internal/repository/postgres"
	"github.com/nexusguard/bot/internal/usecase"
	tele "gopkg.in/telebot.v3"
)

// Handler holds all Telegram bot handlers and services.
type Handler struct {
	svc       *usecase.GroupService
	adminSvc  *usecase.AdminService
	adminRepo *postgres.AdminRepository

	delMu     sync.Mutex
	delTimers map[string]*time.Timer
}

func NewHandler(svc *usecase.GroupService, adminSvc *usecase.AdminService, adminRepo *postgres.AdminRepository) *Handler {
	return &Handler{
		svc:       svc,
		adminSvc:  adminSvc,
		adminRepo: adminRepo,
		delTimers: make(map[string]*time.Timer),
	}
}

// RegisterAll attaches all handlers to the bot.
func (h *Handler) RegisterAll(b *tele.Bot) {
	// Global middleware to reset menu timers on interaction
	b.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			if c.Callback() != nil && c.Message() != nil {
				// Reset the menu's auto-delete timer to 30 seconds upon any interaction
				h.autoDeleteAfter(c.Bot(), c.Message(), 30*time.Second)
			}
			return next(c)
		}
	})

	// ── Private chat ──────────────────────────────────────────────────────
	b.Handle("/start", h.onStart)
	b.Handle("/ping", h.onPing)
	b.Handle("/mygroups", h.onMyGroups)
	b.Handle("/help", h.onHelp)

	// ── Private inline buttons ────────────────────────────────────────────
	b.Handle(&btnStatus, h.onBtnStatus)
	b.Handle(&btnProfile, h.onBtnProfile)
	b.Handle(&btnAddBot, h.onBtnAddBot)
	b.Handle(&btnMyGroups, h.onBtnMyGroups)
	b.Handle(&btnHelp, h.onBtnHelp)
	b.Handle(&btnBack, h.onBack)

	// ── Private group dashboard buttons ───────────────────────────────────
	b.Handle(&btnManageGroup, h.onManageGroup)
	b.Handle(&btnGroupSettings, h.onGroupSettings)
	b.Handle(&btnGroupMembers, h.onGroupMembers)
	b.Handle(&btnGroupAdmins, h.onGroupAdmins)
	b.Handle(&btnGroupWarned, h.onGroupWarned)
	b.Handle(&btnGroupMuted, h.onGroupMuted)
	b.Handle(&btnGroupBanned, h.onGroupBanned)

	// ── Group moderation (role-protected) ─────────────────────────────────
	b.Handle("/warn", h.onWarn)              // Moderator+
	b.Handle("/settings", h.onSettings)      // In group: notifies to use PV
	b.Handle("/addadmin", h.onAddAdmin)      // Owner only
	b.Handle("/addmod", h.onAddMod)          // Admin+
	b.Handle("/removeadmin", h.onRemoveRole) // Owner only
	b.Handle("/admins", h.onListAdmins)      // Moderator+

	// ── Member management (role-protected) ────────────────────────────────
	h.RegisterMemberHandlers(b) // /banned /warned /muted /members /unmute /unban

	// ── Settings toggle callbacks ─────────────────────────────────────────
	b.Handle(&btnToggleLinks, h.onToggleLinks)
	b.Handle(&btnToggleProfanity, h.onToggleProfanity)
	b.Handle(&btnToggleWelcome, h.onToggleWelcome)
	b.Handle(&btnSettingsBack, h.onSettingsBack)

	// ── Group events ──────────────────────────────────────────────────────
	b.Handle(tele.OnUserJoined, h.onUserJoined)
	b.Handle(tele.OnUserLeft, h.onUserLeft)
	b.Handle(tele.OnText, h.onText)
	b.Handle(tele.OnMyChatMember, h.onMyChatMember)
}

// ─── Button definitions ───────────────────────────────────────────────────────
var (
	btnStatus   = tele.Btn{Unique: "btn_status"}
	btnProfile  = tele.Btn{Unique: "btn_profile"}
	btnAddBot   = tele.Btn{Unique: "btn_addbot"}
	btnMyGroups = tele.Btn{Unique: "btn_mygroups"}
	btnHelp     = tele.Btn{Unique: "btn_help"}
	btnBack     = tele.Btn{Unique: "btn_back"}

	// Group management dashboard buttons in PV
	btnManageGroup   = tele.Btn{Unique: "btn_grp_manage"}
	btnGroupSettings = tele.Btn{Unique: "btn_grp_settings"}
	btnGroupMembers  = tele.Btn{Unique: "btn_grp_members"}
	btnGroupAdmins   = tele.Btn{Unique: "btn_grp_admins"}
	btnGroupWarned   = tele.Btn{Unique: "btn_grp_warned"}
	btnGroupMuted    = tele.Btn{Unique: "btn_grp_muted"}
	btnGroupBanned   = tele.Btn{Unique: "btn_grp_banned"}

	btnToggleLinks     = tele.Btn{Unique: "btn_toggle_links"}
	btnToggleProfanity = tele.Btn{Unique: "btn_toggle_profanity"}
	btnToggleWelcome   = tele.Btn{Unique: "btn_toggle_welcome"}
	btnSettingsBack    = tele.Btn{Unique: "btn_settings_back"}
)

// ─── Keyboard Definitions ──────────────────────────────────────────────────────

var (
	// Persistent bottom menu for private chat
	replyMenu = &tele.ReplyMarkup{ResizeKeyboard: true}
	btnHome   = replyMenu.Text("🏠 Main Menu")
)

// ─── /start ──────────────────────────────────────────────────────────────────
func (h *Handler) onStart(c tele.Context) error {
	if c.Chat().Type != tele.ChatPrivate {
		return nil
	}

	user := c.Sender()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.svc.RegisterUser(ctx, &domain.User{
		TelegramID: user.ID, Username: user.Username,
		FirstName: user.FirstName, LastName: user.LastName,
	})

	name := user.FirstName
	if name == "" {
		name = user.Username
	}

	// Delete the command message so it doesn't clutter the chat
	if c.Message() != nil {
		_ = c.Bot().Delete(c.Message())
	}

	// ── Auth check: only owners of a group can access the management panel ──
	isOwner := h.adminRepo.IsOwnerOfAnyGroup(ctx, user.ID)
	if !isOwner {
		// Non-owner: show minimal message — add bot to group to get started
		botUsername := c.Bot().Me.Username
		addURL := fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername)
		text := fmt.Sprintf(
			"🛡️ *NexusGuard*\n\n"+
				"Hi *%s*! To use this bot you need to add it to your group as an administrator.\n\n"+
				"Once added, you'll automatically become the group Owner and can manage everything from here.",
			name,
		)
		menu := &tele.ReplyMarkup{}
		menu.Inline(menu.Row(menu.URL("➕ Add Bot to My Group", addURL)))
		msg, err := c.Bot().Send(c.Sender(), text, menu, tele.ModeMarkdown)
		if err == nil {
			h.autoDeleteAfter(c.Bot(), msg, 30*time.Second)
		}
		return err
	}

	// ── Owner: show full management menu ──
	text := fmt.Sprintf(
		"🛡️ *Welcome to NexusGuard, %s!*\n\n"+
			"• 🔗 Smart spam & link filtering\n"+
			"• ⚠️ Advanced warning, mute & ban system\n"+
			"• ⚙️ Full control & settings via private chat\n"+
			"• 🔐 Multi-tier permissions (Owner / Admin / Moderator)\n"+
			"• 👋 Automated smart welcome messages\n"+
			"• 🎮 XP & activity tracking\n\n"+
			"_This menu auto-deletes in 20s_ 🧹\n\n"+
			"Select an option below ⬇️", name,
	)

	botUsername := c.Bot().Me.Username
	menu := mainMenu(botUsername)
	msg, err := c.Bot().Send(c.Sender(), text, menu, tele.ModeMarkdown)
	if err == nil {
		h.autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	}
	return err
}

func mainMenu(botUsername string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	addURL := fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername)
	menu.Inline(
		menu.Row(
			menu.Data("📡 System Status", btnStatus.Unique),
			menu.Data("👤 My Profile", btnProfile.Unique),
		),
		menu.Row(
			menu.Data("🏘️ My Groups", btnMyGroups.Unique),
			menu.Data("❓ Help", btnHelp.Unique),
		),
		menu.Row(
			menu.URL("➕ Add Bot to Group", addURL),
		),
	)
	return menu
}

func (h *Handler) onPing(c tele.Context) error {
	if c.Chat().Type != tele.ChatPrivate {
		return nil
	}
	return c.Send("🏓 *Pong!* NexusGuard is active and ready.", tele.ModeMarkdown)
}

func (h *Handler) onHelp(c tele.Context) error {
	if c.Chat().Type != tele.ChatPrivate {
		return nil
	}
	text := "📖 *NexusGuard User Guide*\n\n" +
		"*🔐 Permission Management (in group):*\n" +
		"• `/addadmin` — Grant bot admin role (Owner only)\n" +
		"• `/addmod` — Grant moderator role (Admin+)\n" +
		"• `/removeadmin` — Revoke permissions (Owner only)\n" +
		"• `/admins` — List bot administrators\n\n" +
		"*⚠️ Moderation Commands (in group or private):*\n" +
		"• `/warn` — Issue warning (reply to user)\n" +
		"• `/unmute` — Lift mute instantly (reply to user)\n" +
		"• `/unban` — Lift ban instantly (reply to user)\n\n" +
		"*📋 Lists & Analytics:*\n" +
		"• `/warned` — List warned users\n" +
		"• `/muted` — List muted users\n" +
		"• `/banned` — List banned users\n" +
		"• `/members` — List tracked members\n\n" +
		"*⚙️ Group Settings:*\n" +
		"Security settings are managed in private chat under *My Groups*.\n\n" +
		"_Help messages auto-delete in 20s_ 🧹"
	return c.Send(text, tele.ModeMarkdown)
}

// ─── Private button callbacks ────────────────────────────────────────────────

func (h *Handler) onBtnStatus(c tele.Context) error {
	if !h.requirePrivateAuth(c) {
		return answerUnauthorizedCallback(c)
	}
	text := "📡 *NexusGuard System Status*\n\n" +
		"✅ Telegram API: Connected & Active\n" +
		"✅ PostgreSQL: Connected & Healthy\n" +
		"🟢 Bot Status: Online\n\n" +
		fmt.Sprintf("🕐 Server Time: `%s`", time.Now().Format("2006-01-02 15:04:05"))
	return c.Edit(text, backMenu(), tele.ModeMarkdown)
}

func (h *Handler) onBtnProfile(c tele.Context) error {
	if !h.requirePrivateAuth(c) {
		return answerUnauthorizedCallback(c)
	}
	user := c.Sender()
	uname := "—"
	if user.Username != "" {
		uname = "@" + user.Username
	}
	text := fmt.Sprintf(
		"👤 *User Profile*\n\n"+
			"🪪 Name: *%s %s*\n"+
			"🔗 Username: `%s`\n"+
			"🆔 User ID: `%d`",
		user.FirstName, user.LastName, uname, user.ID,
	)
	return c.Edit(text, backMenu(), tele.ModeMarkdown)
}

func (h *Handler) onBtnAddBot(c tele.Context) error {
	if !h.requirePrivateAuth(c) {
		return answerUnauthorizedCallback(c)
	}
	botUsername := c.Bot().Me.Username
	addURL := fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername)
	text := "➕ *Add NexusGuard to Your Group*\n\n" +
		"1. Click the button below.\n" +
		"2. Select your group.\n" +
		"3. Grant the following admin permissions:\n" +
		"   • Delete Messages\n" +
		"   • Restrict Members\n" +
		"   • Ban Users"
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.URL("➕ Select Group", addURL)),
		menu.Row(menu.Data("🔙 Back", btnBack.Unique)),
	)
	return c.Edit(text, menu, tele.ModeMarkdown)
}

func (h *Handler) onBtnMyGroups(c tele.Context) error {
	if !h.requirePrivateAuth(c) {
		return answerUnauthorizedCallback(c)
	}
	return h.showMyGroups(c)
}

func (h *Handler) showMyGroups(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Only show groups where this user is the Owner (not just any admin/moderator)
	groups, err := h.svc.GetOwnerGroups(ctx, c.Sender().ID)
	botUsername := c.Bot().Me.Username
	addURL := fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername)

	if err != nil || len(groups) == 0 {
		text := "🏘️ *My Groups*\n\n" +
			"No groups found where you are the Owner.\n" +
			"Add the bot as an administrator to your group to manage it here."
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.URL("➕ Add to Group", addURL)),
			menu.Row(menu.Data("🔙 Back to Menu", btnBack.Unique)),
		)
		return c.Edit(text, menu, tele.ModeMarkdown)
	}

	// ── Verify each group via Telegram API — deactivate stale ones ──
	var activeGroups []domain.Group
	for _, g := range groups {
		chat, err := c.Bot().ChatByID(g.TelegramID)
		if err != nil || chat == nil {
			// Bot is no longer in this group — mark inactive
			slog.Info("Group no longer accessible, marking inactive",
				"group_id", g.TelegramID, "title", g.Title)
			_ = h.svc.SetGroupActive(ctx, g.ID, false)
			continue
		}
		// Update title if it changed
		if chat.Title != "" && chat.Title != g.Title {
			g.Title = chat.Title
		}
		g.IsActive = true
		activeGroups = append(activeGroups, g)
	}

	if len(activeGroups) == 0 {
		text := "🏘️ *My Groups*\n\n" +
			"No active groups found. Your previous groups are no longer accessible.\n" +
			"Add the bot to a new group to get started."
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.URL("➕ Add to Group", addURL)),
			menu.Row(menu.Data("🔙 Back to Menu", btnBack.Unique)),
		)
		return c.Edit(text, menu, tele.ModeMarkdown)
	}

	text := fmt.Sprintf("🏘️ *My Groups* (%d groups)\n\nSelect a group to manage members, admins, and security settings:", len(activeGroups))
	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	callerID := c.Sender().ID
	for _, g := range activeGroups {
		title := g.Title
		if title == "" {
			title = fmt.Sprintf("Group %d", g.TelegramID)
		}
		btnLabel := fmt.Sprintf("🟢 🛡️ %s", truncate(title, 22))
		// Embed callerID in callback data: "<groupTelegramID>:<callerTelegramID>"
		// This ensures only the person who opened the menu can interact with it
		data := fmt.Sprintf("%d:%d", g.TelegramID, callerID)
		rows = append(rows, menu.Row(
			menu.Data(btnLabel, btnManageGroup.Unique, data),
		))
	}

	rows = append(rows, menu.Row(
		menu.URL("➕ Add to Another Group", addURL),
	))
	rows = append(rows, menu.Row(
		menu.Data("🔙 Back to Main Menu", btnBack.Unique),
	))

	menu.Inline(rows...)
	return c.Edit(text, menu, tele.ModeMarkdown)
}

func (h *Handler) onBtnHelp(c tele.Context) error {
	if !h.requirePrivateAuth(c) {
		return answerUnauthorizedCallback(c)
	}
	return h.onHelp(c)
}

func (h *Handler) onBack(c tele.Context) error {
	if !h.requirePrivateAuth(c) {
		return answerUnauthorizedCallback(c)
	}
	user := c.Sender()
	name := user.FirstName
	if name == "" {
		name = user.Username
	}
	text := fmt.Sprintf("🛡️ *Welcome to NexusGuard, %s!*\n\nUse the menu below:", name)
	botUsername := c.Bot().Me.Username
	return c.Edit(text, mainMenu(botUsername), tele.ModeMarkdown)
}

func (h *Handler) onMyGroups(c tele.Context) error {
	if !h.requirePrivateAuth(c) {
		return nil
	}
	return h.showMyGroups(c)
}

func backMenu() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("🔙 Back", btnBack.Unique)))
	return menu
}

// ─── onMyChatMember — Bot added to group ───────────────────────────────────────
// Sends notification to adder in private chat only. Does not send any message in group.
func (h *Handler) onMyChatMember(c tele.Context) error {
	chat := c.Chat()
	update := c.ChatMember()
	if update == nil {
		return nil
	}

	newStatus := update.NewChatMember.Role
	slog.Info("Bot status changed in chat",
		"chat_id", chat.ID, "title", chat.Title, "new_status", newStatus,
	)

	// Only when added (member or admin)
	if newStatus != tele.Member && newStatus != tele.Administrator && newStatus != tele.Creator {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adderID := int64(0)
	adderUsername := ""
	if update.Sender != nil {
		adderID = update.Sender.ID
		adderUsername = update.Sender.Username
	}

	// Register group in database
	group, err := h.svc.RegisterGroup(ctx, chat.ID, chat.Title, chat.Username, adderID)
	if err != nil {
		slog.Error("Failed to register group", "error", err)
		return nil
	}

	// Grant Owner role to adder
	if adderID != 0 {
		_ = h.adminSvc.GrantOwner(ctx, group.ID, adderID, adderUsername)
	}

	// ─── Send private message to the adder ───
	if adderID != 0 && update.Sender != nil {
		adminNote := ""
		if newStatus != tele.Administrator {
			adminNote = "\n\n⚠️ *For full functionality:*\nGrant me administrator rights (delete messages + restrict members)."
		}
		privateText := fmt.Sprintf(
			"✅ *NexusGuard added to \"%s\"!*\n\n"+
				"👑 You have been registered as *Owner*.\n\n"+
				"*Commands:*\n"+
				"• `/addadmin @user` — Bot Admin\n"+
				"• `/addmod @user` — Moderator\n"+
				"• `/settings` — Settings\n"+
				"• `/admins` — Admin list%s",
			chat.Title, adminNote,
		)
		adder := update.Sender
		_, _ = c.Bot().Send(adder, privateText, tele.ModeMarkdown)
	}

	return nil
}

// ─── onUserJoined — New user joined ──────────────────────────────────────────
func (h *Handler) onUserJoined(c tele.Context) error {
	chat := c.Chat()
	bot := c.Bot().Me

	// If bot itself was added, do nothing (handled by onMyChatMember)
	for _, m := range c.Message().UsersJoined {
		if m.ID == bot.ID {
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, chat.ID)
	if err != nil || !group.WelcomeEnabled {
		return nil
	}

	for _, member := range c.Message().UsersJoined {
		name := member.FirstName
		if name == "" {
			name = member.Username
		}
		msg := group.WelcomeMessage
		if msg == "" {
			msg = fmt.Sprintf(
				"👋 *Welcome, %s!*\n\nWelcome to *%s* 🎉\nPlease follow group rules 🛡️",
				name, chat.Title,
			)
		}
		sentMsg, err := c.Bot().Send(c.Chat(), msg, tele.ModeMarkdown)
		if err == nil {
			h.autoDeleteAfter(c.Bot(), sentMsg, 30*time.Second)
		}
	}
	return nil
}

// ─── onUserLeft — User left or was removed ───────────────────────────────────
func (h *Handler) onUserLeft(c tele.Context) error {
	chat := c.Chat()
	left := c.Message().UserLeft
	if left == nil || left.IsBot {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, chat.ID)
	if err != nil {
		return nil // Group not registered — nothing to clean up
	}

	// Remove ALL bot roles (owner/admin/moderator) for this user in this group
	if err := h.adminRepo.PurgeUserFromGroup(ctx, group.ID, left.ID); err != nil {
		slog.Error("Failed to purge user roles on leave",
			"user_id", left.ID, "group_id", group.ID, "error", err)
	}

	// Soft-delete membership record
	if err := h.svc.SoftDeleteMember(ctx, group.ID, left.ID); err != nil {
		slog.Error("Failed to soft-delete member on leave",
			"user_id", left.ID, "group_id", group.ID, "error", err)
	}

	slog.Info("User left/removed — roles and membership purged",
		"user_id", left.ID,
		"username", left.Username,
		"chat_id", chat.ID,
		"group_title", chat.Title,
	)

	return nil
}

// ─── onText — Handle text messages ───────────────────────────────────────────
func (h *Handler) onText(c tele.Context) error {
	// ── Private Chat Text ──────────────────────────────────────────
	if c.Chat().Type == tele.ChatPrivate {
		if c.Message().Text == btnHome.Text {
			_ = c.Bot().Delete(c.Message())
			name := c.Sender().FirstName
			if name == "" {
				name = c.Sender().Username
			}
			text := fmt.Sprintf("🛡️ *Welcome to NexusGuard, %s!*\n\nSelect an option from the menu below:", name)
			return c.Send(text, mainMenu(c.Bot().Me.Username), tele.ModeMarkdown)
		}
		return nil
	}

	// ── Group Chat Text ────────────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}

	shouldDelete, reason := h.svc.ShouldFilterMessage(ctx, group, c.Message().Text)
	if shouldDelete {
		slog.Info("Deleting spam message",
			"chat_id", c.Chat().ID, "user_id", c.Sender().ID, "reason", reason,
		)
		_ = c.Delete()
		msg, _ := c.Bot().Send(c.Chat(), fmt.Sprintf("⛔ Message deleted: _%s_", reason), tele.ModeMarkdown)
		if msg != nil {
			h.autoDeleteAfter(c.Bot(), msg, 5*time.Second)
		}
		return nil
	}
	go h.svc.AddXPForMessage(context.Background(), group, c.Sender().ID)
	return nil
}

// ─── /warn ───────────────────────────────────────────────────────────────────
func (h *Handler) onWarn(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		deleteCommandMsg(c)
		return nil // Group not registered — silent
	}

	// Check permission — Admin or above (also deletes the command msg)
	if !h.requireGroupRole(c, group.ID, postgres.RoleAdmin) {
		return nil // Silent for unauthorized users
	}

	replied := c.Message().ReplyTo
	if replied == nil {
		msg, _ := c.Bot().Send(c.Chat(), "⚠️ Reply to a user message and type `/warn`.", tele.ModeMarkdown)
		if msg != nil {
			h.autoDeleteAfter(c.Bot(), msg, 10*time.Second)
		}
		return nil
	}

	target := replied.Sender
	if target == nil || target.IsBot {
		return nil
	}

	args := c.Args()
	reason := "Violation of group rules"
	if len(args) > 0 {
		reason = strings.Join(args, " ")
	}

	warnCount, action, err := h.svc.WarnUser(ctx, group, target.ID, c.Sender().ID, reason)
	if err != nil {
		slog.Error("WarnUser failed", "error", err)
		return nil
	}

	targetName := target.FirstName
	if target.Username != "" {
		targetName = "@" + target.Username
	}

	var responseText string
	switch action {
	case "ban":
		// Ban in Telegram — removes user from group permanently
		_ = c.Bot().Ban(c.Chat(), &tele.ChatMember{User: target}, true)
		// ✅ Sync DB flag so /banned list shows this user
		_ = h.svc.SetBanStatus(context.Background(), group.ID, target.ID, true)
		responseText = fmt.Sprintf(
			"🚫 *%s was banned!*\n\n"+
				"📋 Reason: _%s_\n"+
				"⚠️ Warnings: %d of %d\n"+
				"🔒 Status: Permanent ban\n"+
				"🕐 Time: `%s`",
			targetName, reason, warnCount, group.MaxWarns*2,
			time.Now().Format("2006-01-02 15:04"),
		)
	case "mute":
		muteEnd := time.Now().Add(time.Duration(group.MuteDuration) * time.Minute)
		muteUntilUnix := muteEnd.Unix()
		// Restrict in Telegram — no messages, no media, no stickers
		_ = c.Bot().Restrict(c.Chat(), &tele.ChatMember{
			User:            target,
			Rights:          tele.NoRights(),
			RestrictedUntil: muteUntilUnix,
		})
		// ✅ Sync DB flags so /muted list shows this user
		_ = h.svc.SetMuteStatus(context.Background(), group.ID, target.ID, true, &muteEnd)
		responseText = fmt.Sprintf(
			"🔇 *%s was muted!*\n\n"+
				"📋 Reason: _%s_\n"+
				"⚠️ Warnings: %d of %d\n"+
				"⏱ Duration: %d minutes\n"+
				"🔓 Unmute at: `%s`",
			targetName, reason, warnCount, group.MaxWarns,
			group.MuteDuration,
			muteEnd.Format("2006-01-02 15:04"),
		)
	default:
		responseText = fmt.Sprintf(
			"⚠️ *Warning %d for %s*\n\n"+
				"📋 Reason: _%s_\n"+
				"📊 Progress: %s",
			warnCount, targetName, reason,
			buildWarnProgressBar(warnCount, group.MaxWarns),
		)
	}

	msg, _ := c.Bot().Send(c.Chat(), responseText, tele.ModeMarkdown)
	if msg != nil {
		h.autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	}
	return nil
}

// buildWarnProgressBar renders a visual warn progress bar like: 🟥🟥🟥⬜⬜ (3/5)
func buildWarnProgressBar(count, max int) string {
	if max <= 0 {
		max = 3
	}
	filled := count
	if filled > max {
		filled = max
	}
	bar := strings.Repeat("🟥", filled) + strings.Repeat("⬜", max-filled)
	return fmt.Sprintf("%s `[%d/%d]`", bar, count, max)
}

// ─── /settings ───────────────────────────────────────────────────────────────
func (h *Handler) onSettings(c tele.Context) error {
	if c.Chat().Type != tele.ChatPrivate {
		deleteCommandMsg(c)

		// In group: only Moderator+ gets the PM link, everyone else is silently ignored
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		group, err := h.svc.GetGroup(ctx, c.Chat().ID)
		if err != nil {
			return nil // Group not registered — silent
		}
		if !h.requireGroupRole(c, group.ID, postgres.RoleModerator) {
			return nil // Unauthorized — silent (requireGroupRole already deleted the msg)
		}

		botUsername := c.Bot().Me.Username
		pvURL := fmt.Sprintf("https://t.me/%s?start=groups", botUsername)
		menu := &tele.ReplyMarkup{}
		menu.Inline(
			menu.Row(menu.URL("⚙️ Manage Settings in PM", pvURL)),
		)
		msg, err := c.Bot().Send(c.Chat(),
			"🔒 *Security settings are managed in private chat.*\n"+
				"To keep group chat clean and secure, please configure settings in private:",
			menu, tele.ModeMarkdown,
		)
		if err == nil && msg != nil {
			h.autoDeleteAfter(c.Bot(), msg, 15*time.Second)
		}
		return nil
	}
	return h.showMyGroups(c)
}

func settingsText(g *domain.Group) string {
	on := func(b bool) string {
		if b {
			return "✅ Enabled"
		}
		return "❌ Disabled"
	}
	return fmt.Sprintf(
		"⚙️ *Group Settings: %s*\n\n"+
			"🔗 Link Filter: %s\n"+
			"🤬 Profanity Filter: %s\n"+
			"👋 Welcome Message: %s\n"+
			"⚠️ Max Warnings: `%d`\n"+
			"🔇 Mute Duration: `%d minutes`\n\n"+
			"_Click buttons below to toggle settings:_",
		g.Title, on(g.FilterLinks), on(g.FilterProfanity), on(g.WelcomeEnabled),
		g.MaxWarns, g.MuteDuration,
	)
}

func settingsMenu(g *domain.Group, callerID int64) *tele.ReplyMarkup {
	linkLabel := fmt.Sprintf("🔗 Link Filter: %s", map[bool]string{true: "✅", false: "❌"}[g.FilterLinks])
	profLabel := fmt.Sprintf("🤬 Profanity Filter: %s", map[bool]string{true: "✅", false: "❌"}[g.FilterProfanity])
	wlcLabel := fmt.Sprintf("👋 Welcome Message: %s", map[bool]string{true: "✅", false: "❌"}[g.WelcomeEnabled])
	// Use dashboard data format to carry callerID through toggle callbacks
	data := buildDashboardData(g.TelegramID, callerID)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data(linkLabel, btnToggleLinks.Unique, data)),
		menu.Row(menu.Data(profLabel, btnToggleProfanity.Unique, data)),
		menu.Row(menu.Data(wlcLabel, btnToggleWelcome.Unique, data)),
		menu.Row(menu.Data("🔙 Back to Group Panel", btnSettingsBack.Unique, data)),
	)
	return menu
}

func (h *Handler) onToggleLinks(c tele.Context) error {
	return h.toggleSetting(c, func(g *domain.Group) { g.FilterLinks = !g.FilterLinks })
}
func (h *Handler) onToggleProfanity(c tele.Context) error {
	return h.toggleSetting(c, func(g *domain.Group) { g.FilterProfanity = !g.FilterProfanity })
}
func (h *Handler) onToggleWelcome(c tele.Context) error {
	return h.toggleSetting(c, func(g *domain.Group) { g.WelcomeEnabled = !g.WelcomeEnabled })
}

func (h *Handler) toggleSetting(c tele.Context, toggle func(*domain.Group)) error {
	// Use parseDashboardCallback so callerID is verified
	groupTelegramID, authorized := parseDashboardCallback(c)
	if !authorized {
		return answerUnauthorizedCallback(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Failed to retrieve group"})
	}

	// Only the Owner may change settings
	if group.OwnerID != c.Sender().ID {
		return answerUnauthorizedCallback(c)
	}

	toggle(group)
	if err := h.svc.UpdateSettings(ctx, group); err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Failed to save settings"})
	}
	_ = c.Respond(&tele.CallbackResponse{Text: "✅ Saved"})
	return c.Edit(settingsText(group), settingsMenu(group, c.Sender().ID), tele.ModeMarkdown)
}

func (h *Handler) onSettingsBack(c tele.Context) error {
	return h.onManageGroup(c)
}

// ─── Secure callback data helpers ────────────────────────────────────────────

// parseDashboardCallback parses "<groupTelegramID>:<callerTelegramID>" callback data.
// Returns (groupTelegramID, authorized) where authorized is true only if the presser
// is the same user who opened the dashboard.
func parseDashboardCallback(c tele.Context) (groupTelegramID int64, authorized bool) {
	data := c.Data()
	var gID, cID int64
	n, err := fmt.Sscanf(data, "%d:%d", &gID, &cID)
	if err != nil || n != 2 {
		return 0, false
	}
	return gID, c.Sender().ID == cID
}

// buildDashboardData builds "<groupTelegramID>:<callerTelegramID>" callback data.
func buildDashboardData(groupTelegramID, callerID int64) string {
	return fmt.Sprintf("%d:%d", groupTelegramID, callerID)
}

// ─── Private Group Dashboard Handlers ────────────────────────────────────────

func (h *Handler) onManageGroup(c tele.Context) error {
	groupTelegramID, authorized := parseDashboardCallback(c)
	if !authorized {
		// Not the person who opened the menu — silently ignore
		return answerUnauthorizedCallback(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	// Verify sender is actually the owner of this group
	if group.OwnerID != c.Sender().ID {
		return answerUnauthorizedCallback(c)
	}

	statusStr := "🟢 Active"
	if !group.IsActive {
		statusStr = "🔴 Inactive"
	}
	filterLinksStr := "❌ Disabled"
	if group.FilterLinks {
		filterLinksStr = "✅ Enabled"
	}
	filterProfanityStr := "❌ Disabled"
	if group.FilterProfanity {
		filterProfanityStr = "✅ Enabled"
	}
	welcomeStr := "❌ Disabled"
	if group.WelcomeEnabled {
		welcomeStr = "✅ Enabled"
	}

	text := fmt.Sprintf(
		"🛡️ *Group Management Panel: %s*\n\n"+
			"🆔 ID: `%d`\n"+
			"📊 Status: %s\n"+
			"🔗 Link Filter: %s\n"+
			"🤬 Profanity Filter: %s\n"+
			"👋 Welcome Message: %s\n"+
			"⚠️ Max Warnings: `%d`\n"+
			"🔇 Mute Duration: `%d minutes`\n\n"+
			"Click options below to manage group ⬇️",
		group.Title, group.TelegramID, statusStr,
		filterLinksStr, filterProfanityStr, welcomeStr,
		group.MaxWarns, group.MuteDuration,
	)

	callerID := c.Sender().ID
	data := buildDashboardData(group.TelegramID, callerID)
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("⚙️ Security Settings", btnGroupSettings.Unique, data)),
		menu.Row(
			menu.Data("👥 Members List", btnGroupMembers.Unique, data),
			menu.Data("🛡️ Admin List", btnGroupAdmins.Unique, data),
		),
		menu.Row(
			menu.Data("⚠️ Warned", btnGroupWarned.Unique, data),
			menu.Data("🔇 Muted", btnGroupMuted.Unique, data),
		),
		menu.Row(
			menu.Data("🚫 Banned", btnGroupBanned.Unique, data),
		),
		menu.Row(
			menu.Data("🔙 Back to My Groups", btnMyGroups.Unique),
		),
	)

	_ = c.Respond()
	return c.Edit(text, menu, tele.ModeMarkdown)
}

func (h *Handler) onGroupSettings(c tele.Context) error {
	groupTelegramID, authorized := parseDashboardCallback(c)
	if !authorized {
		return answerUnauthorizedCallback(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	if group.OwnerID != c.Sender().ID {
		return answerUnauthorizedCallback(c)
	}

	_ = c.Respond()
	return c.Edit(settingsText(group), settingsMenu(group, c.Sender().ID), tele.ModeMarkdown)
}

func (h *Handler) onGroupMembers(c tele.Context) error {
	groupTelegramID, authorized := parseDashboardCallback(c)
	if !authorized {
		return answerUnauthorizedCallback(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	if group.OwnerID != c.Sender().ID {
		return answerUnauthorizedCallback(c)
	}

	members, err := h.svc.ListAllMembers(ctx, group.ID, 25)
	var sb strings.Builder
	fmt.Fprintf(&sb, "👥 *Known Members: %s*\n\n", group.Title)
	if err != nil || len(members) == 0 {
		sb.WriteString("No members recorded yet.\n_Members are recorded when they send messages._")
	} else {
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
				flags += fmt.Sprintf(" ⚠️%d", m.WarnCount)
			}
			fmt.Fprintf(&sb, "%d. %s %s\n", i+1, name, flags)
		}
	}

	data := buildDashboardData(group.TelegramID, c.Sender().ID)
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("🔙 Back to Group Panel", btnManageGroup.Unique, data)))

	_ = c.Respond()
	return c.Edit(sb.String(), menu, tele.ModeMarkdown)
}

func (h *Handler) onGroupAdmins(c tele.Context) error {
	groupTelegramID, authorized := parseDashboardCallback(c)
	if !authorized {
		return answerUnauthorizedCallback(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	if group.OwnerID != c.Sender().ID {
		return answerUnauthorizedCallback(c)
	}

	admins, err := h.adminSvc.ListAdmins(ctx, group.ID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "🛡️ *NexusGuard Admins: %s*\n\n", group.Title)
	if err != nil || len(admins) == 0 {
		sb.WriteString("No admins registered for this group yet.")
	} else {
		roleEmoji := map[postgres.BotRole]string{
			postgres.RoleOwner:     "👑",
			postgres.RoleAdmin:     "🛡️",
			postgres.RoleModerator: "🔧",
		}
		for _, a := range admins {
			emoji := roleEmoji[a.Role]
			name := a.Username
			if name != "" {
				name = "@" + name
			} else {
				name = fmt.Sprintf("user_%d", a.TelegramID)
			}
			fmt.Fprintf(&sb, "%s %s — `%s`\n", emoji, name, a.Role)
		}
	}

	data := buildDashboardData(group.TelegramID, c.Sender().ID)
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("🔙 Back to Group Panel", btnManageGroup.Unique, data)))

	_ = c.Respond()
	return c.Edit(sb.String(), menu, tele.ModeMarkdown)
}

func (h *Handler) onGroupWarned(c tele.Context) error {
	groupTelegramID, authorized := parseDashboardCallback(c)
	if !authorized {
		return answerUnauthorizedCallback(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	if group.OwnerID != c.Sender().ID {
		return answerUnauthorizedCallback(c)
	}

	members, _ := h.svc.ListWarnedMembers(ctx, group.ID)
	data := buildDashboardData(group.TelegramID, c.Sender().ID)
	markup := &tele.ReplyMarkup{}
	backRow := markup.Row(markup.Data("🔙 Back to Group Panel", btnManageGroup.Unique, data))
	text, menu := buildWarnedList(members, group.TelegramID, group.MaxWarns, backRow)

	_ = c.Respond()
	return c.Edit(text, menu, tele.ModeMarkdown)
}

func (h *Handler) onGroupMuted(c tele.Context) error {
	groupTelegramID, authorized := parseDashboardCallback(c)
	if !authorized {
		return answerUnauthorizedCallback(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	if group.OwnerID != c.Sender().ID {
		return answerUnauthorizedCallback(c)
	}

	members, _ := h.svc.ListMutedMembers(ctx, group.ID)
	data := buildDashboardData(group.TelegramID, c.Sender().ID)
	markup := &tele.ReplyMarkup{}
	backRow := markup.Row(markup.Data("🔙 Back to Group Panel", btnManageGroup.Unique, data))
	text, menu := buildMutedList(members, group.TelegramID, backRow)

	_ = c.Respond()
	return c.Edit(text, menu, tele.ModeMarkdown)
}

func (h *Handler) onGroupBanned(c tele.Context) error {
	groupTelegramID, authorized := parseDashboardCallback(c)
	if !authorized {
		return answerUnauthorizedCallback(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, groupTelegramID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Group not found"})
	}

	if group.OwnerID != c.Sender().ID {
		return answerUnauthorizedCallback(c)
	}

	members, _ := h.svc.ListBannedMembers(ctx, group.ID)
	data := buildDashboardData(group.TelegramID, c.Sender().ID)
	markup := &tele.ReplyMarkup{}
	backRow := markup.Row(markup.Data("🔙 Back to Group Panel", btnManageGroup.Unique, data))
	text, menu := buildBannedList(members, group.TelegramID, backRow)

	_ = c.Respond()
	return c.Edit(text, menu, tele.ModeMarkdown)
}

// ─── /addadmin ───────────────────────────────────────────────────────────────
func (h *Handler) onAddAdmin(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	deleteCommandMsg(c)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}

	// Only Owner can add admins
	if !h.requireOwnerOnly(c, group.ID) {
		return nil
	}

	target, errMsg := h.extractTarget(c)
	if errMsg != "" {
		msg, _ := c.Bot().Send(c.Chat(), errMsg, tele.ModeMarkdown)
		if msg != nil {
			h.autoDeleteAfter(c.Bot(), msg, 10*time.Second)
		}
		return nil
	}

	if err := h.adminSvc.AddAdmin(ctx, group.ID, c.Sender().ID, target.ID, target.Username); err != nil {
		return nil
	}

	name := target.FirstName
	if target.Username != "" {
		name = "@" + target.Username
	}
	msg, _ := c.Bot().Send(c.Chat(),
		fmt.Sprintf("🛡️ *%s* has been registered as *Admin*.", name), tele.ModeMarkdown)
	if msg != nil {
		h.autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	}
	return nil
}

// ─── /addmod ─────────────────────────────────────────────────────────────────
func (h *Handler) onAddMod(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	deleteCommandMsg(c)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}

	if !h.requireGroupRole(c, group.ID, postgres.RoleAdmin) {
		return nil
	}

	target, errMsg := h.extractTarget(c)
	if errMsg != "" {
		msg, _ := c.Bot().Send(c.Chat(), errMsg, tele.ModeMarkdown)
		if msg != nil {
			h.autoDeleteAfter(c.Bot(), msg, 10*time.Second)
		}
		return nil
	}

	if err := h.adminSvc.AddModerator(ctx, group.ID, c.Sender().ID, target.ID, target.Username); err != nil {
		return nil
	}

	name := target.FirstName
	if target.Username != "" {
		name = "@" + target.Username
	}
	msg, _ := c.Bot().Send(c.Chat(),
		fmt.Sprintf("🔧 *%s* has been registered as *Moderator*.", name), tele.ModeMarkdown)
	if msg != nil {
		h.autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	}
	return nil
}

// ─── /removeadmin ────────────────────────────────────────────────────────────
func (h *Handler) onRemoveRole(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	deleteCommandMsg(c)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}

	if !h.requireOwnerOnly(c, group.ID) {
		return nil
	}

	target, errMsg := h.extractTarget(c)
	if errMsg != "" {
		msg, _ := c.Bot().Send(c.Chat(), errMsg, tele.ModeMarkdown)
		if msg != nil {
			h.autoDeleteAfter(c.Bot(), msg, 10*time.Second)
		}
		return nil
	}

	if err := h.adminSvc.RemoveRole(ctx, group.ID, c.Sender().ID, target.ID); err != nil {
		return nil
	}

	name := target.FirstName
	if target.Username != "" {
		name = "@" + target.Username
	}
	msg, _ := c.Bot().Send(c.Chat(),
		fmt.Sprintf("🗑️ Permissions removed for *%s*.", name), tele.ModeMarkdown)
	if msg != nil {
		h.autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	}
	return nil
}

// ─── /admins ─────────────────────────────────────────────────────────────────
func (h *Handler) onListAdmins(c tele.Context) error {
	if c.Chat().Type == tele.ChatPrivate {
		return nil
	}
	deleteCommandMsg(c)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	group, err := h.svc.GetGroup(ctx, c.Chat().ID)
	if err != nil {
		return nil
	}

	if !h.requireGroupRole(c, group.ID, postgres.RoleModerator) {
		return nil
	}

	admins, err := h.adminSvc.ListAdmins(ctx, group.ID)
	if err != nil || len(admins) == 0 {
		msg, _ := c.Bot().Send(c.Chat(), "📋 No admins or moderators configured.", tele.ModeMarkdown)
		if msg != nil {
			h.autoDeleteAfter(c.Bot(), msg, 15*time.Second)
		}
		return nil
	}

	roleEmoji := map[postgres.BotRole]string{
		postgres.RoleOwner:     "👑",
		postgres.RoleAdmin:     "🛡️",
		postgres.RoleModerator: "🔧",
	}

	var sb strings.Builder
	sb.WriteString("📋 *NexusGuard Admin List*\n\n")
	for _, a := range admins {
		emoji := roleEmoji[a.Role]
		name := a.Username
		if name != "" {
			name = "@" + name
		} else {
			name = fmt.Sprintf("user_%d", a.TelegramID)
		}
		fmt.Fprintf(&sb, "%s %s — `%s`\n", emoji, name, a.Role)
	}
	sb.WriteString("\n_This message will self-destruct in 20 seconds_ 🧹")

	msg, _ := c.Bot().Send(c.Chat(), sb.String(), tele.ModeMarkdown)
	if msg != nil {
		h.autoDeleteAfter(c.Bot(), msg, 20*time.Second)
	}
	return nil
}

// ─── Helper: extract target from reply or args ────────────────────────────────
func (h *Handler) extractTarget(c tele.Context) (*tele.User, string) {
	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		return c.Message().ReplyTo.Sender, ""
	}
	return nil, "⚠️ Please reply to the target user's message."
}

// autoDeleteAfter deletes a message after the given duration.
// Safely resets the timer if called again for the same message.
func (h *Handler) autoDeleteAfter(bot *tele.Bot, msg *tele.Message, delay time.Duration) {
	if msg == nil || bot == nil {
		return
	}
	key := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.ID)

	h.delMu.Lock()
	defer h.delMu.Unlock()

	if t, exists := h.delTimers[key]; exists {
		t.Stop()
	}

	h.delTimers[key] = time.AfterFunc(delay, func() {
		h.delMu.Lock()
		delete(h.delTimers, key)
		h.delMu.Unlock()
		_ = bot.Delete(msg)
	})
}
