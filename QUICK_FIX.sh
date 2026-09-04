#!/bin/bash
# Quick fix script for /admins command

echo "🔍 Checking NexusGuard database..."

# Check if bot is running
if ! docker ps | grep -q nexusguard_bot; then
    echo "❌ Bot is not running! Starting..."
    docker compose up -d
    sleep 3
fi

echo ""
echo "📊 Database Status:"
echo "=================="

# List all groups
echo ""
echo "Groups in database:"
docker exec nexusguard_db psql -U $(grep DB_USER .env | cut -d'=' -f2) -d nexusguard -c "SELECT id, telegram_id, title, owner_id FROM groups;"

echo ""
echo "Bot admins in database:"
docker exec nexusguard_db psql -U $(grep DB_USER .env | cut -d'=' -f2) -d nexusguard -c "SELECT ba.id, ba.group_id, ba.telegram_id, ba.username, ba.role, g.title as group_name FROM group_bot_admins ba JOIN groups g ON g.id = ba.group_id;"

echo ""
echo "=================="
echo ""
echo "✅ To fix /admins issue:"
echo "1. Remove bot from your group"
echo "2. Re-add bot as ADMIN (important!)"
echo "3. Check logs: docker compose logs -f bot | grep Owner"
echo "4. Try /admins again in your group"
echo ""
echo "📋 Current bot logs (last 20 lines):"
docker compose logs --tail=20 bot
