#!/usr/bin/env bash
# Publish Calden's updater-compatible alpha WITHOUT GitHub Actions.
set -Eeuo pipefail
REPO=gigabytegrove/calden
BRANCH=alpha/v1.5.0-alpha.1
VERSION=1.5.0-alpha.1
TAG=v${VERSION}
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
die(){ printf 'ERROR: %s\n' "$*" >&2; exit 1; }
log(){ printf '\n========== %s ==========\n' "$*"; }
for program in docker git gh tar sha256sum grep mktemp; do
  command -v "$program" >/dev/null 2>&1 || die "Missing executable: $program"
done
[[ "$(tr -d '[:space:]' < VERSION)" == "$VERSION" ]] || die "VERSION mismatch"
[[ "$(git branch --show-current)" == "$BRANCH" ]] || die "Wrong branch; use $BRANCH"
[[ -z "$(git status --porcelain)" ]] || die "Working tree must be clean"
docker info >/dev/null 2>&1 || die "Docker daemon unavailable"
gh auth status >/dev/null 2>&1 || die "Authenticate gh with a PAT (gh auth login or GH_TOKEN)"
HEAD=$(git rev-parse HEAD)
REMOTE=$(gh api "repos/$REPO/git/ref/heads/$BRANCH" --jq '.object.sha') ||
  die "Cannot verify alpha branch on GitHub"
[[ "$REMOTE" == "$HEAD" ]] || die "Local alpha branch differs from GitHub; pull before publishing."
if gh release view "$TAG" -R "$REPO" >/dev/null 2>&1; then
  die "Release $TAG already exists; refusing to overwrite."
fi
if gh api "repos/$REPO/git/ref/tags/$TAG" >/dev/null 2>&1; then
  die "Tag $TAG already exists; refusing to overwrite."
fi

WORK=$(mktemp -d)
cleanup(){ rm -rf "$WORK"; }
trap cleanup EXIT
mkdir -p "$WORK/dist" "$WORK/verify" "$WORK/web"

log "Build Tailwind web bundle AND run all Node tests"
docker build --target webbuild --build-arg CALDEN_RUN_TESTS=1 \
  --tag "calden-alpha-webbuild:$VERSION" .
CID=$(docker create "calden-alpha-webbuild:$VERSION")
if ! docker cp "$CID:/src/web/." "$WORK/web"; then
  docker rm -f "$CID" >/dev/null 2>&1 || true
  die "Could not extract web build"
fi
docker rm -f "$CID" >/dev/null
[[ -s "$WORK/web/index.html" && -s "$WORK/web/styles.css" ]] ||
  die "Web entry page or built CSS missing"
grep -Fq "name=\"calden-version\" content=\"$VERSION\"" "$WORK/web/index.html" ||
  die "Built UI version mismatch"

for arch in amd64 arm64; do
  log "Go unit tests and Linux $arch build"
  docker build --target build --build-arg CALDEN_RUN_TESTS=1 \
    --build-arg TARGETOS=linux --build-arg TARGETARCH="$arch" \
    --tag "calden-alpha-go-$arch:$VERSION" .
  CID=$(docker create "calden-alpha-go-$arch:$VERSION")
  mkdir -p "$WORK/stage/$arch"
  if ! docker cp "$CID:/out/calden" "$WORK/stage/$arch/calden"; then
    docker rm -f "$CID" >/dev/null 2>&1 || true
    die "Could not extract Linux $arch binary"
  fi
  docker rm -f "$CID" >/dev/null
  chmod 0755 "$WORK/stage/$arch/calden"
  cp -a "$WORK/web" "$WORK/stage/$arch/web"
  (cd "$WORK/stage/$arch" &&
     tar -czf "$WORK/dist/calden-linux-$arch.tar.gz" calden web)
  contents=$(tar -tzf "$WORK/dist/calden-linux-$arch.tar.gz")
  printf '%s\n' "$contents" | grep -Fxq 'calden' ||
    die "$arch archive lacks calden executable"
  printf '%s\n' "$contents" | grep -Fxq 'web/index.html' ||
    die "$arch archive lacks index.html"
  printf '%s\n' "$contents" | grep -Fxq 'web/styles.css' ||
    die "$arch archive lacks compiled stylesheet"
done

log "Build checksums and validate runtime archives"
(cd "$WORK/dist" &&
  sha256sum calden-linux-amd64.tar.gz calden-linux-arm64.tar.gz > SHA256SUMS &&
  sha256sum -c SHA256SUMS)

log "Publish GitHub prerelease/tag with verified updater assets"
gh release create "$TAG" \
  "$WORK/dist/calden-linux-amd64.tar.gz" \
  "$WORK/dist/calden-linux-arm64.tar.gz" \
  "$WORK/dist/SHA256SUMS" \
  --repo "$REPO" --target "$HEAD" \
  --title "Calden $VERSION — Alpha" \
  --notes-file "docs/releases/$TAG.md" \
  --prerelease --latest=false

log "Verify released assets from GitHub"
gh release download "$TAG" -R "$REPO" -D "$WORK/verify" \
  --pattern 'calden-linux-amd64.tar.gz' \
  --pattern 'calden-linux-arm64.tar.gz' \
  --pattern 'SHA256SUMS'
(cd "$WORK/verify" && sha256sum -c SHA256SUMS)
[[ "$(gh release view "$TAG" -R "$REPO" --json isPrerelease --jq '.isPrerelease')" == true ]] ||
  die "Release is not flagged as prerelease"
[[ "$(gh release view "$TAG" -R "$REPO" --json isDraft --jq '.isDraft')" == false ]] ||
  die "Release remained draft"
printf '\nSUCCESS — %s published and updater assets verified.\n' "$TAG"
printf 'https://github.com/%s/releases/tag/%s\n' "$REPO" "$TAG"
