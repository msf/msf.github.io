#!/bin/bash
set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../" && pwd)"
cd "$REPO_ROOT"

echo "🚀 Building with Hugo..."
~/bin/hugo --source "$REPO_ROOT" --destination "$REPO_ROOT/public"

echo "✅ Build complete. Output in $REPO_ROOT/public"

echo "📦 Deploying to /srv/selfhost/blog/site/..."
rsync -av --delete "$REPO_ROOT/public/" /srv/selfhost/blog/site/

echo "🎉 Deployment successful!"
