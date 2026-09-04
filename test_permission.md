# 🔴 Security Bug Report: Unauthorized Access

## Problem
Regular users (without roles) might access protected commands:
- `/settings` (should NOT have access!)
- `/unban` (should NOT have access!)

## Root Cause Analysis

### Scenario 1: Command Permission Check
```go
// handlers.go:573
if !h.requireGroupRole(c, group.ID, postgres.RoleAdmin) {
    return nil  // Works correctly
}
```

### Scenario 2: Callback Permission Check  
```go
// handlers.go:631
group, err := h.svc.GetGroup(ctx, c.Message().Chat.ID)  // 🔴 PROBLEM!
```

**Issue:** In callback queries:
- `c.Message()` might be the ORIGINAL message (when /settings was sent)
- `c.Message().Chat` might be nil or point to wrong chat
- Permission check happens on WRONG chat ID!

## Test Cases

### Test 1: Regular user tries /settings in group
**Expected:** ❌ Command deleted, no response
**Actual:** Need to verify

### Test 2: Regular user clicks settings button
**Expected:** ❌ Silent rejection
**Actual:** ✅ Might work! (BUG)

### Test 3: Regular user tries /unban via reply
**Expected:** ❌ Command deleted, no response  
**Actual:** Need to verify

## Possible Bugs

### Bug 1: Callback Chat ID Resolution
```go
// ❌ WRONG
group, err := h.svc.GetGroup(ctx, c.Message().Chat.ID)

// ✅ CORRECT
// Callbacks need to encode chat_id in callback data!
```

### Bug 2: Missing Permission Check
Some handlers might not have permission check at all!

## Action Items

1. ✅ Review ALL handler functions
2. ✅ Ensure EVERY command/callback has permission check
3. ✅ Fix callback chat ID resolution
4. ✅ Add integration tests for permission system
