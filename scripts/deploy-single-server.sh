#!/usr/bin/env bash
set -euo pipefail

# Build the client image locally, upload it to one SSH host, load it into Docker,
# update the target Compose file, and restart the client service.
#
# Required:
#   SSH_CONTEXT=<ssh-host> ./scripts/deploy-single-server.sh
#
# Optional:
#   REMOTE_WORK_DIR=/opt/wechatread
#   IMAGE_NAME=wechatread-client:local
#   SERVICE_NAME=client
#   BUILD_PLATFORM=linux/amd64
#   LOCAL_TARBALL=/tmp/wechatread-client-local.tar.gz
#   REMOTE_TARBALL=/tmp/wechatread-client-local.tar.gz
#   FORCE=1

if [[ -z "${SSH_CONTEXT:-}" ]]; then
  echo "SSH_CONTEXT is required, for example: SSH_CONTEXT=my-server $0" >&2
  exit 2
fi

REMOTE_WORK_DIR="${REMOTE_WORK_DIR:-/opt/wechatread}"
IMAGE_NAME="${IMAGE_NAME:-wechatread-client:local}"
SERVICE_NAME="${SERVICE_NAME:-client}"
BUILD_PLATFORM="${BUILD_PLATFORM:-linux/amd64}"
LOCAL_TARBALL="${LOCAL_TARBALL:-/tmp/wechatread-client-local.tar.gz}"
REMOTE_TARBALL="${REMOTE_TARBALL:-/tmp/wechatread-client-local.tar.gz}"
FORCE="${FORCE:-0}"

if [[ "$FORCE" != "1" ]]; then
  local_head="$(git rev-parse HEAD 2>/dev/null || echo not-a-git-repo)"
  remote_head="$(ssh "$SSH_CONTEXT" "cd '$REMOTE_WORK_DIR/client' 2>/dev/null && git rev-parse HEAD 2>/dev/null || echo not-a-git-repo")"
  if [[ "$remote_head" != "not-a-git-repo" && "$local_head" != "$remote_head" ]]; then
    cat >&2 <<EOF
Local and remote git HEAD differ.
local:  $local_head
remote: $remote_head

Push/pull first, or rerun with FORCE=1 if you intentionally deploy this local tree.
EOF
    exit 1
  fi
fi

echo "Building $IMAGE_NAME for $BUILD_PLATFORM"
docker buildx build --platform "$BUILD_PLATFORM" --load -t "$IMAGE_NAME" .

echo "Saving image to $LOCAL_TARBALL"
docker save "$IMAGE_NAME" | gzip > "$LOCAL_TARBALL"
ls -lh "$LOCAL_TARBALL"

echo "Uploading image to $SSH_CONTEXT:$REMOTE_TARBALL"
scp "$LOCAL_TARBALL" "$SSH_CONTEXT:$REMOTE_TARBALL"

echo "Loading image and updating Compose service"
ssh "$SSH_CONTEXT" "set -euo pipefail
docker load < '$REMOTE_TARBALL'
cd '$REMOTE_WORK_DIR'
if grep -q 'image: wechatread-client:' docker-compose.yml; then
  sed -i 's|image: wechatread-client:.*|image: $IMAGE_NAME|' docker-compose.yml
fi
docker compose up -d '$SERVICE_NAME'
rm -f '$REMOTE_TARBALL'
docker compose ps '$SERVICE_NAME'
"

echo "Deployment complete"
