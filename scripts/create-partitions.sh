#!/bin/bash
# Automatic partition creation script (T125)
# Run monthly to create partitions for upcoming months

set -e

# Configuration
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-pod_game}"
DB_USER="${DB_USER:-postgres}"
MONTHS_AHEAD=3  # Create partitions for next 3 months

echo "Creating partitions for next $MONTHS_AHEAD months..."
echo "Database: $DB_NAME on $DB_HOST:$DB_PORT"

# Function to create partition for a given month
create_partition() {
    local year=$1
    local month=$2
    local partition_name="game_events_${year}_$(printf "%02d" $month)"

    # Calculate start and end dates
    local start_date="${year}-$(printf "%02d" $month)-01"

    # Calculate next month
    local next_month=$((month + 1))
    local next_year=$year
    if [ $next_month -gt 12 ]; then
        next_month=1
        next_year=$((year + 1))
    fi
    local end_date="${next_year}-$(printf "%02d" $next_month)-01"

    echo "Creating partition: $partition_name ($start_date to $end_date)"

    # Check if partition already exists
    local exists=$(PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -tAc \
        "SELECT EXISTS (SELECT FROM pg_tables WHERE tablename = '$partition_name');")

    if [ "$exists" = "t" ]; then
        echo "  Partition $partition_name already exists, skipping"
        return 0
    fi

    # Create partition
    PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME <<SQL
CREATE TABLE IF NOT EXISTS $partition_name PARTITION OF game_events
    FOR VALUES FROM ('$start_date') TO ('$end_date');
SQL

    if [ $? -eq 0 ]; then
        echo "  ✓ Created partition $partition_name"
    else
        echo "  ✗ Failed to create partition $partition_name"
        return 1
    fi
}

# Get current date
current_year=$(date +%Y)
current_month=$(date +%m | sed 's/^0*//')  # Remove leading zero

# Create partitions for upcoming months
for i in $(seq 1 $MONTHS_AHEAD); do
    # Calculate target month
    target_month=$((current_month + i))
    target_year=$current_year

    # Handle year rollover
    while [ $target_month -gt 12 ]; do
        target_month=$((target_month - 12))
        target_year=$((target_year + 1))
    done

    # Create partition
    create_partition $target_year $target_month
done

echo "Partition creation completed!"

# Optional: Clean up old partitions (older than retention period)
# Uncomment to enable automatic partition cleanup
# RETENTION_MONTHS=12
# cutoff_month=$((current_month - RETENTION_MONTHS))
# cutoff_year=$current_year
# while [ $cutoff_month -le 0 ]; do
#     cutoff_month=$((cutoff_month + 12))
#     cutoff_year=$((cutoff_year - 1))
# done
# old_partition="game_events_${cutoff_year}_$(printf "%02d" $cutoff_month)"
# echo "Dropping old partition: $old_partition"
# PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "DROP TABLE IF EXISTS $old_partition;"
