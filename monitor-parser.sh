#!/bin/bash
# Monitor parser activity

echo "=== Monitoring Parser Activity ==="
echo ""
echo "Press Ctrl+C to stop"
echo ""

while true; do
    clear
    echo "=== $(date) ==="
    echo ""

    echo "--- Database State ---"
    PGPASSWORD='myAwEsOm3pa55@w0rd' psql -h localhost -p 5433 -U user -d db -c \
        "SELECT last_processed_lt, last_poll_timestamp FROM blockchain_sync_state WHERE id=1;" \
        2>/dev/null

    echo ""
    echo "--- Game Count ---"
    PGPASSWORD='myAwEsOm3pa55@w0rd' psql -h localhost -p 5433 -U user -d db -c \
        "SELECT COUNT(*) as total_games, MAX(game_id) as latest_game_id FROM games;" \
        2>/dev/null

    echo ""
    echo "--- Recent Events ---"
    PGPASSWORD='myAwEsOm3pa55@w0rd' psql -h localhost -p 5433 -U user -d db -c \
        "SELECT game_id, event_type, timestamp FROM game_events ORDER BY timestamp DESC LIMIT 3;" \
        2>/dev/null

    sleep 5
done
