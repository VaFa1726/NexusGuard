# 🔍 Debug Guide: /admins Not Showing

## Possible Issues

### Issue 1: Group Not in Database
**Check:**
```sql
SELECT id, telegram_id, title, owner_id FROM groups;
```
**Expected:** At least one row with your group

---

### Issue 2: No Admins Registered
**Check:**
```sql
SELECT * FROM group_bot_admins;
```
**Expected:** At least one row (the Owner)

---

### Issue 3: Group ID Mismatch
**Check:**
```sql
-- Find group internal ID
SELECT id FROM groups WHERE telegram_id = <YOUR_GROUP_TELEGRAM_ID>;

-- Check admins for that group
SELECT * FROM group_bot_admins WHERE group_id = <INTERNAL_GROUP_ID>;
```

---

## Manual Database Check

```bash
# Connect to database
docker exec -it nexusguard_db psql -U <DB_USER> -d nexusguard

# List all groups
SELECT id, telegram_id, title, owner_id, created_at FROM groups;

# List all bot admins
SELECT ba.id, ba.group_id, ba.telegram_id, ba.username, ba.role, 
       g.title as group_name
FROM group_bot_admins ba
JOIN groups g ON g.id = ba.group_id;

# Check specific group
SELECT * FROM group_bot_admins WHERE group_id = 1;
```

---

## Fix Steps

### Step 1: Verify Bot Was Added to Group
1. Remove bot from group
2. Re-add bot as ADMIN (important!)
3. Check logs for: "Bot status changed in chat"
4. Check logs for: "Owner granted"

### Step 2: Manually Add Owner (if missing)
In the bot logs, when you add the bot to a group, you should see:
```
Bot status changed in chat | chat_id=<ID> | new_status=administrator
Group registered | group_id=<ID>
Owner granted | group_db_id=<ID> | telegram_id=<OWNER_ID>
```

If you DON'T see "Owner granted", the issue is in `onMyChatMember` handler.

### Step 3: Test /admins Again
```
/admins
```

**Expected Response:**
```
📋 NexusGuard Admin List in this group

👑 @username — owner

Total: 1
This message will self-destruct in 20 seconds 🧹
```

---

## Debug Logs to Check

When you run `/admins`, check the bot logs for:

### Success Path:
```
No logs (silent success)
```

### Error Path:
```
/admins failed: group not found | chat_id=<ID> | error=...
```
OR
```
/admins failed to list | group_id=<ID> | error=...
```

---

## Common Mistakes

### Mistake 1: Bot Not Admin
If bot is added as regular member (not admin), it can't be fully tracked.
**Fix:** Promote bot to admin

### Mistake 2: Bot Removed and Re-added
If bot was removed and re-added, the group might have stale data.
**Fix:** Check `groups` table for the correct group entry

### Mistake 3: Multiple Groups with Same Name
If you have multiple test groups, make sure you're checking the right one.
**Fix:** Use `telegram_id` (chat ID) to identify groups

---

## Expected Behavior After Fix

1. Add bot to group → Owner automatically registered ✅
2. `/admins` → Shows owner ✅
3. `/addadmin @user` → Adds admin, `/admins` shows both ✅
4. `/addmod @user` → Adds moderator, `/admins` shows all 3 ✅
