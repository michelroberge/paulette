#!/bin/sh
# Run as root: fix permissions on repos that may have been written by root
# (common when the host directory is bind-mounted and dolt created files as root).
find /home/paulette/repos -name 'dolt-server.*' -exec chmod 666 {} \; 2>/dev/null || true
find /home/paulette/repos -name 'dolt' -type d -exec chmod 755 {} \; 2>/dev/null || true
chown -R paulette:paulette /home/paulette/repos 2>/dev/null || true

exec gosu paulette /usr/local/bin/paulette "$@"
