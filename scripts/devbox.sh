#!/usr/bin/env sh
# devbox.sh — headless-провижинер devbox: up | down | recreate из канон-манифеста
# (Foundation Шаг 2, briefs/devbox-provision-lifecycle.md). Любой продукт поднимается/
# пересоздаётся ОДНОЙ командой из своего `.devcontainer/devcontainer.json`, по канону.
#
#   scripts/devbox.sh up        [repo-root]   # создать (если нет) + запустить; идемпотентно
#   scripts/devbox.sh down      [repo-root]   # остановить + удалить контейнер (volumes НЕ трогаем)
#   scripts/devbox.sh recreate  [repo-root]   # down + up; данные (secrets/pnpm/рантайм-volume) переживают
#
# containers-only: на ХОСТЕ нужен ТОЛЬКО docker (node/git может не быть). Манифест парсит
# node ВНУТРИ контейнера образа (devbox-manifest.mjs) — единый источник, ноль дублирования.
# Канон-ИНВАРИАНТ (network=omnifield-gateway alias=repo, --restart unless-stopped, единственный
# bind своего репо, ноль host-портов) ставит этот скрипт; продукт-переменное (image/env/volumes/
# hooks) — из манифеста. Ноль per-продукт-развилок, ноль ручных `docker run`.
#
# Тест-хук (ТОЛЬКО для CI/дев-проверки, в проде не используется): DEVBOX_EMITTER_LOCAL=1 —
# запускать эмиттер хостовым node вместо контейнера; DEVBOX_DRY_RUN=1 — печатать docker-команды,
# не исполнять.
set -eu

CMD="${1:-}"
[ "$#" -gt 0 ] && shift || true
[ -n "$CMD" ] || { echo "usage: devbox.sh <up|down|recreate> [repo-root]" >&2; exit 64; }

command -v docker >/dev/null 2>&1 || { echo "✖ нужен docker в PATH (containers-only)" >&2; exit 1; }

REPO_ROOT=$(cd "${1:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}" && pwd)
REPO=$(basename "$REPO_ROOT")
CONTAINER="${REPO}-devbox"
MANIFEST="$REPO_ROOT/.devcontainer/devcontainer.json"
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
NETWORK="omnifield-gateway"
DRY="${DEVBOX_DRY_RUN:-}"

[ -f "$MANIFEST" ] || { echo "✖ нет манифеста $MANIFEST — devbox провижинится из .devcontainer/devcontainer.json" >&2; exit 1; }

# docker-исполнитель с dry-run (печатает вместо запуска).
dk() {
  if [ -n "$DRY" ]; then
    printf 'docker'
    for a in "$@"; do printf ' %s' "$a"; done
    printf '\n'
    return 0
  fi
  docker "$@"
}

# image-поле — bootstrap sed'ом (нужно, чтобы запустить эмиттер в контейнере ДО парсинга).
IMAGE=$(sed -n 's/^[[:space:]]*"image"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$MANIFEST" | head -n1)
[ -n "$IMAGE" ] || { echo "✖ в манифесте не найдено поле image" >&2; exit 1; }

# Эмиттер: в проде — node в контейнере образа (репо ro-mount'ом); в тесте — хостовый node.
emit() {
  if [ -n "${DEVBOX_EMITTER_LOCAL:-}" ]; then
    node "$SCRIPT_DIR/devbox-manifest.mjs" "$1" "$MANIFEST" ${2:+"$2"}
  else
    docker run --rm -v "$REPO_ROOT:/repo:ro" "$IMAGE" \
      node /repo/scripts/devbox-manifest.mjs "$1" /repo/.devcontainer/devcontainer.json ${2:+"$2"}
  fi
}

exists() { docker inspect "$CONTAINER" >/dev/null 2>&1; }
running() { [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null || echo false)" = "true" ]; }

ensure_network() {
  docker network inspect "$NETWORK" >/dev/null 2>&1 || {
    echo "[devbox] создаю внешнюю docker-сеть $NETWORK (один раз на машину)…"
    dk network create "$NETWORK" >/dev/null
  }
}

run_hook() {  # $1=hook-поле $2=строгий(1)/мягкий
  hook=$(emit hook "$1")
  [ -n "$hook" ] || return 0
  echo "[devbox] $1…"
  if [ "$2" = "strict" ]; then
    dk exec "$CONTAINER" sh -c "$hook"
  else
    dk exec "$CONTAINER" sh -c "$hook" || echo "[devbox] $1 не прошёл (мягкий) — продолжаю"
  fi
}

create_container() {
  echo "[devbox] создаю $CONTAINER из манифеста (канон-инвариант ставит провизионер)…"
  # Канон-инвариант (НЕ из манифеста) + продукт-аргументы из эмиттера (по токену на строку;
  # docker-аргументы переводов строк не содержат). POSIX: читаем построчно в позиционные.
  set --
  while IFS= read -r tok; do
    [ -n "$tok" ] && set -- "$@" "$tok"
  done <<EOF
$(emit create-args)
EOF
  dk create \
    --name "$CONTAINER" \
    --restart unless-stopped \
    --network "$NETWORK" \
    --network-alias "$REPO" \
    --mount "type=bind,source=$REPO_ROOT,target=/workspaces/$REPO" \
    -w "/workspaces/$REPO" \
    "$@" >/dev/null
}

do_up() {
  ensure_network
  if exists; then
    if running; then
      echo "[devbox] $CONTAINER уже поднят — no-op (идемпотентно)."
      return 0
    fi
    echo "[devbox] $CONTAINER есть, но остановлен — docker start…"
    dk start "$CONTAINER" >/dev/null
    run_hook postStartCommand soft
    return 0
  fi
  create_container
  dk start "$CONTAINER" >/dev/null
  run_hook postCreateCommand strict
  run_hook postStartCommand soft
  echo "[devbox] $CONTAINER поднят. Вход: scripts/devbox-session.sh <scope>"
}

do_down() {
  if exists; then
    echo "[devbox] удаляю $CONTAINER (volumes/данные не трогаю — переживут recreate)…"
    dk rm -f "$CONTAINER" >/dev/null
  else
    echo "[devbox] $CONTAINER уже нет — no-op."
  fi
}

case "$CMD" in
  up) do_up ;;
  down) do_down ;;
  recreate) do_down; do_up ;;
  *) echo "usage: devbox.sh <up|down|recreate> [repo-root]" >&2; exit 64 ;;
esac
