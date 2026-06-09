#!/bin/bash
set -e

REMOTE="root@192.168.8.4"
BACKUP_DIR="/opt/karaxxx/backups"
DB_PATH="/opt/karaxxx/karaxxx.db"
TIMESTAMP=$(date +%Y%m%d-%H%M)
BACKUP_FILE="karaxxx.db.${TIMESTAMP}"

ssh "${REMOTE}" "mkdir -p ${BACKUP_DIR}"
ssh "${REMOTE}" "sqlite3 ${DB_PATH} \".backup ${BACKUP_DIR}/${BACKUP_FILE}\""
ssh "${REMOTE}" "gzip ${BACKUP_DIR}/${BACKUP_FILE}"
ssh "${REMOTE}" "ls -t ${BACKUP_DIR}/karaxxx.db.*.gz | tail -n +8 | xargs -r rm"

echo "Backup created: ${BACKUP_FILE}.gz"
