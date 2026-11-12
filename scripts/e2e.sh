#!/bin/sh
# End-to-end test of a built dotf binary in a throwaway HOME:
#   install -> status -> unapply -> restore
# Usage: scripts/e2e.sh ./dotf
set -eu

DOTF=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

export HOME="$WORK/home"
export XDG_STATE_HOME="$WORK/state"
unset DOTF_ALLOW_HOOKS || true
REPO="$WORK/repo"
mkdir -p "$HOME" "$REPO/layers/base/.config/kitty" "$REPO/layers/base/hooks" "$REPO/layers/base/packages"

fail() { echo "FAIL: $*" >&2; exit 1; }
step() { echo; echo "== $*"; }

printf 'base zshrc\n' > "$REPO/layers/base/.zshrc"
printf 'font_size 12\n' > "$REPO/layers/base/.config/kitty/kitty.conf"
printf 'touch "$DOTF_HOME/hook-ran"\n' > "$REPO/layers/base/hooks/post-install.sh"
printf 'definitely-not-a-package\n' > "$REPO/layers/base/packages/apt.txt"

# A file the user already has, with private permissions.
printf 'my own zshrc\n' > "$HOME/.zshrc"
chmod 600 "$HOME/.zshrc"

step "version"
"$DOTF" version | grep -q '^dotf ' || fail "version not printed on stdout"

step "explain"
(cd "$REPO" && "$DOTF" explain)

step "install --dry-run writes nothing"
(cd "$REPO" && "$DOTF" install --dry-run --allow-hooks)
[ ! -e "$XDG_STATE_HOME/dotf" ] || fail "dry run created the state directory"
[ ! -L "$HOME/.zshrc" ] || fail "dry run replaced .zshrc"
[ ! -e "$HOME/hook-ran" ] || fail "dry run ran a hook"
[ ! -e "$REPO/dotf.yaml" ] || fail "dotf.yaml was generated"

step "install"
"$DOTF" install --no-packages "$REPO"
[ -L "$HOME/.zshrc" ] || fail ".zshrc is not a symlink"
[ -L "$HOME/.config/kitty/kitty.conf" ] || fail "kitty.conf is not a symlink"
[ ! -e "$HOME/hooks" ] || fail "hooks/ was linked into HOME"
[ ! -e "$HOME/packages" ] || fail "packages/ was linked into HOME"
[ ! -e "$HOME/hook-ran" ] || fail "hook ran without --allow-hooks"
ls -A "$XDG_STATE_HOME/dotf/backups/" | grep -q '^\.zshrc@' || fail "no backup of the original .zshrc"
[ "$(stat -c %a "$XDG_STATE_HOME/dotf")" = 700 ] || fail "state directory is not 0700"

step "install again is a no-op"
"$DOTF" install --no-packages "$REPO" 2>&1 | grep -q 'unchanged' || fail "second install changed files"

step "status (from outside the repository)"
(cd / && "$DOTF" status)
(cd / && "$DOTF" status --json) > "$WORK/status.json"
grep -q '"ok": 2' "$WORK/status.json" || fail "status --json does not report 2 OK files"

step "status exits 1 when a file is missing"
rm "$HOME/.config/kitty/kitty.conf"
if "$DOTF" status --json > /dev/null 2>&1; then fail "status --json exited 0 with a missing file"; fi

step "doctor --fix recreates it"
if "$DOTF" doctor > /dev/null; then fail "doctor exited 0 with a missing file"; fi
"$DOTF" doctor --fix
[ -L "$HOME/.config/kitty/kitty.conf" ] || fail "doctor --fix did not recreate kitty.conf"

step "unapply"
"$DOTF" unapply
[ ! -e "$HOME/.zshrc" ] || fail ".zshrc still present after unapply"
[ ! -e "$HOME/.config/kitty/kitty.conf" ] || fail "kitty.conf still present after unapply"

step "restore"
"$DOTF" restore --preview
"$DOTF" restore .zshrc
[ "$(cat "$HOME/.zshrc")" = "my own zshrc" ] || fail "restored .zshrc has the wrong content"
[ "$(stat -c %a "$HOME/.zshrc")" = 600 ] || fail "restored .zshrc lost its 0600 mode"
if "$DOTF" restore .does-not-exist 2>/dev/null; then fail "restore of a missing backup exited 0"; fi

step "exit codes"
set +e
"$DOTF" unapply > /dev/null 2>&1; code=$?
[ "$code" = 3 ] || fail "unapply with nothing installed: exit $code, want 3"
"$DOTF" no-such-command > /dev/null 2>&1; code=$?
[ "$code" = 64 ] || fail "unknown command: exit $code, want 64"
set -e

echo
echo "e2e OK"
