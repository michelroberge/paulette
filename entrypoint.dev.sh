#!/bin/sh
# Fix dolt file permissions inside the repos volume (same as production entrypoint).
# Use -maxdepth 2 to avoid descending into bind-mounted project directories (e.g. node_modules).
find /home/paulette/repos -maxdepth 2 -name 'dolt-server.*' -exec chmod 666 {} \; 2>/dev/null || true
find /home/paulette/repos -maxdepth 2 -name 'dolt' -type d -exec chmod 755 {} \; 2>/dev/null || true
# Chown only the volume root, not bind-mounted subdirs (avoids recursing into node_modules etc.)
chown paulette:paulette /home/paulette/repos 2>/dev/null || true

# Ensure air can write the compiled binary into the backend-tmp volume (owned by root on first run)
mkdir -p /app/backend/tmp && chown paulette:paulette /app/backend/tmp

# Ensure paulette owns its data directories (named volumes are root-owned on first init)
chown -R paulette:paulette /home/paulette/.paulette /home/paulette/.beads 2>/dev/null || true

# Go module cache volume also starts root-owned; chown the dir only (not recursive — files
# inside are written by paulette so they'll already be correctly owned after first build)
mkdir -p /go/pkg/mod && chown paulette:paulette /go/pkg/mod 2>/dev/null || true

# Run air as the paulette user for live reload
exec gosu paulette air "$@"
