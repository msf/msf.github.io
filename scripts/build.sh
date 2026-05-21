#!/usr/bin/env bash
# Build the Hugo site and deploy it to the local Caddy-served directory.
#
# Env:
#   BLOG_DEST  Destination directory served by Caddy.
#              Default: /srv/selfhost/blog/site
#   HUGO       Path to the hugo binary. Default: hugo from PATH, else ~/bin/hugo
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="$REPO_ROOT/public"
BLOG_DEST="${BLOG_DEST:-/srv/selfhost/blog/site}"
HUGO="${HUGO:-$(command -v hugo || echo "$HOME/bin/hugo")}"

if [[ ! -x "$HUGO" ]]; then
  echo "error: hugo not found (tried PATH and ~/bin/hugo). Set HUGO=/path/to/hugo." >&2
  exit 1
fi

if [[ ! -d "$BLOG_DEST" ]]; then
  echo "error: BLOG_DEST does not exist: $BLOG_DEST" >&2
  exit 1
fi

echo "==> Building with Hugo ($HUGO)"
"$HUGO" --source "$REPO_ROOT" --destination "$BUILD_DIR"

# Sanity: refuse to rsync --delete from an empty/missing build dir.
if [[ ! -d "$BUILD_DIR" ]] || [[ -z "$(ls -A "$BUILD_DIR" 2>/dev/null)" ]]; then
  echo "error: build dir is empty or missing: $BUILD_DIR — refusing to wipe $BLOG_DEST" >&2
  exit 1
fi

echo "==> Deploying to $BLOG_DEST"
# Why this set of flags:
#   -rltD                  recurse, copy symlinks, preserve file mtimes, devices
#   --no-owner --no-group  don't try to chown/chgrp — destination uses setgid
#                          + default ACLs to keep group ownership consistent
#   --no-perms             don't preserve perms either — let the destination's
#                          default ACL apply (avoids "chmod: Operation not
#                          permitted" when dirs are owned by a different user)
#   --omit-dir-times       don't touch dir mtimes (also avoids spurious errors)
#   --delete               drop files in dest that no longer exist in source
rsync -rltD --delete --no-owner --no-group --no-perms --omit-dir-times \
  "$BUILD_DIR/" "$BLOG_DEST/"

echo "==> Done. $BLOG_DEST is live."
