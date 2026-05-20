#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <version>   (e.g. $0 v1.2.0)" >&2
  exit 1
fi

VERSION="$1"

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must look like vMAJOR.MINOR.PATCH (got '$VERSION')" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "error: working tree is dirty, commit or stash first" >&2
  exit 1
fi

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "$BRANCH" != "main" ]; then
  echo "error: must be on main (currently on '$BRANCH')" >&2
  exit 1
fi

if git rev-parse "$VERSION" >/dev/null 2>&1; then
  echo "error: tag '$VERSION' already exists" >&2
  exit 1
fi

git pull --ff-only origin main
git tag -a "$VERSION" -m "Release $VERSION"
git push origin "$VERSION"

echo "pushed $VERSION — watch the release workflow at:"
echo "  https://github.com/midhun-mohan/glance/actions"
