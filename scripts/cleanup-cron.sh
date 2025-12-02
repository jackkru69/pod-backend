#!/bin/bash
# Data retention cleanup cron script (T124)
# Run monthly to remove old game data

set -e

# Configuration
APP_DIR="/opt/pod-backend"
LOG_DIR="/var/log/pod-backend"
RETENTION_DAYS=365

# Ensure log directory exists
mkdir -p "$LOG_DIR"

# Timestamp for log file
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="$LOG_DIR/cleanup_$TIMESTAMP.log"

echo "Starting data retention cleanup at $(date)" | tee -a "$LOG_FILE"
echo "Retention period: $RETENTION_DAYS days" | tee -a "$LOG_FILE"

# Run cleanup job
cd "$APP_DIR"
./bin/cleanup --retention-days="$RETENTION_DAYS" 2>&1 | tee -a "$LOG_FILE"

# Check exit code
if [ $? -eq 0 ]; then
    echo "Cleanup completed successfully at $(date)" | tee -a "$LOG_FILE"

    # Optional: Send success notification
    # curl -X POST https://your-monitoring-service.com/webhook \
    #   -d "status=success&job=cleanup&timestamp=$(date -Iseconds)"
else
    echo "Cleanup failed at $(date)" | tee -a "$LOG_FILE"

    # Optional: Send failure alert
    # curl -X POST https://your-monitoring-service.com/webhook \
    #   -d "status=failure&job=cleanup&timestamp=$(date -Iseconds)"

    exit 1
fi

# Rotate old logs (keep last 12 months)
find "$LOG_DIR" -name "cleanup_*.log" -mtime +365 -delete

echo "Cleanup job finished at $(date)" | tee -a "$LOG_FILE"
