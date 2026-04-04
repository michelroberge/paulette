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

# Fix .beads ownership (named volume may be created as root)
find /home/paulette/.beads ! -user paulette -exec chown paulette:paulette {} + 2>/dev/null || true

# Fix .ssh ownership and permissions (named volume may be created as root)
if [ -d /home/paulette/.ssh ]; then
  chown -R paulette:paulette /home/paulette/.ssh
  chmod 700 /home/paulette/.ssh
  find /home/paulette/.ssh -type f -name 'id_*' ! -name '*.pub' -exec chmod 600 {} \;
  find /home/paulette/.ssh -type f \( -name '*.pub' -o -name 'known_hosts' -o -name 'authorized_keys' -o -name 'config' \) -exec chmod 644 {} \;
fi

exec gosu paulette /usr/local/bin/paulette "$@"
