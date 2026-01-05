#!/bin/bash
# Backup spatial.json and events.jsonl
# Run via cron: 0 */6 * * * /home/exedev/malten/scripts/backup.sh

BACKUP_DIR="/home/exedev/malten/backups"
DATE=$(date +%Y%m%d_%H%M%S)

# Create timestamped backups
cp /home/exedev/malten/spatial.json "$BACKUP_DIR/spatial_$DATE.json"
cp /home/exedev/malten/events.jsonl "$BACKUP_DIR/events_$DATE.jsonl"

# Compress
gzip "$BACKUP_DIR/spatial_$DATE.json"
gzip "$BACKUP_DIR/events_$DATE.jsonl"

# Keep only last 7 days of backups (28 backups at 6-hour intervals)
find "$BACKUP_DIR" -name "spatial_*.json.gz" -mtime +7 -delete
find "$BACKUP_DIR" -name "events_*.jsonl.gz" -mtime +7 -delete

echo "Backup complete: $DATE"
