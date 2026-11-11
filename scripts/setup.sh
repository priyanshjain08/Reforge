#!/bin/sh
# Creates .env with random secrets (macOS/Linux).
[ -f .env ] && { echo ".env already exists; leaving it unchanged."; exit 0; }
r() { head -c 30 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
A=$(r)
printf 'REFORGE_ADMIN_TOKEN=%s\nREFORGE_VIEWER_TOKEN=\nREFORGE_AGENT_SECRET=%s\nREFORGE_RECOVERY_MODE=approval\n' "$A" "$(r)" > .env
echo "Created .env. Your dashboard login token is: $A"
