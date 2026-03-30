#!/bin/sh
# Run as root: fix permissions on repos that may have been written by root
# (common when the host directory is bind-mounted and dolt created files as root).
find /home/paulette/repos -name 'dolt-server.*' -exec chmod 666 {} \; 2>/dev/null || true
find /home/paulette/repos -name 'dolt' -type d -exec chmod 755 {} \; 2>/dev/null || true
find /home/paulette/repos ! -user paulette -exec chown paulette:paulette {} + 2>/dev/null || true

# Ensure ~/.claude/settings.json exists (volume mount may hide the image copy)
if [ ! -f /home/paulette/.claude/settings.json ]; then
  mkdir -p /home/paulette/.claude
  printf '%s' \
    '{"permissions":{"allow":[' \
    '"Write(/home/paulette/repos/**)",' \
    '"Edit(/home/paulette/repos/**)",' \
    '"Bash(*)",' \
    '"Read(/home/paulette/repos/**)"' \
    ']}}' \
    > /home/paulette/.claude/settings.json
fi
chown -R paulette:paulette /home/paulette/.claude 2>/dev/null || true
find /home/paulette/.paulette ! -user paulette -exec chown paulette:paulette {} + 2>/dev/null || true


exec gosu paulette /usr/local/bin/paulette "$@"
