#!/usr/bin/env sh
# devbox.sh — headless-провижинер devbox под VOLUME-workspace-модель: up | down | recreate
# (DEVOPSER-188, ADR-16 «workspace = named-volume git-clone, всё в Docker»). Один продукт —
# одна команда из его манифеста В ТОМЕ, по канону.
#
#   scripts/devbox.sh up       <repo> [--repo-url URL]  # том+клон (если нужно) + контейнер; идемпотентно
#   scripts/devbox.sh down     <repo>                    # удалить контейнер (том/данные НЕ трогаем)
#   scripts/devbox.sh recreate <repo> [--repo-url URL]   # down + up; том/secrets/pnpm переживают
#
# МОДЕЛЬ. Рабочая копия репо живёт в named-volume `<repo>-workspace` (ext4), НЕ в host-bind папке
# WSL. На ХОСТЕ кода репо больше нет — истина в томе (containers-only). Свежий/пустой том
# наполняется git-клоном (bootstrap-контейнер; creds gitconfig/gh из тома omnifield-secrets), затем
# `chown` на vscode(1000) — свежий named-том root-owned (гага пилота brainer 2026-07-24). Манифест
# ЧИТАЕТСЯ ИЗ ТОМА — провижн не дублирует ручной docker run (гага-дрейф). Гонка host-bind→tmpfs
# (RAM-крах 2026-07-24) схлопывается САМА: том всегда ext4. fail-loud guard ниже — страховка.
#
# РАЗДЕЛЕНИЕ ОТВЕТСТВЕННОСТИ. Канон-ИНВАРИАНТ ставит провизионер (НЕ из манифеста): workspace-том
# → /workspaces/<repo>, network=omnifield-gateway + alias=<repo>, --restart unless-stopped, ноль
# host-портов. Продукт-переменное (image/containerEnv/mounts/hooks) — из манифеста в томе, парсит
# devbox-manifest.mjs. Ноль per-продукт-развилок, ноль ручных docker run.
#
# ТЕСТ-ХУКИ (ТОЛЬКО для CI/дев-проверки, в проде не используются):
#   DEVBOX_DRY_RUN=1      — печатать docker-команды, не исполнять (том/клон/guard тоже печатаются,
#                           docker не трогается — доказательство без хоста, [[no-docker-in-session]]);
#   DEVBOX_EMITTER_LOCAL=1— эмиттер хостовым node из host-манифеста (DEVBOX_LOCAL_MANIFEST или
#                           $PWD/.devcontainer/devcontainer.json), без чтения тома;
#   DEVBOX_BOOTSTRAP_IMAGE— образ для git-clone/эмиттера (дефолт ghcr.io/omnifield/devbox:latest;
#                           несёт git+node, версия для клона нерелевантна — финальный образ из манифеста).
set -eu

CMD="${1:-}"
[ "$#" -gt 0 ] && shift || true
[ -n "$CMD" ] || { echo "usage: devbox.sh <up|down|recreate> <repo> [--repo-url URL]" >&2; exit 64; }

# --- аргументы: <repo> + опц. --repo-url ------------------------------------
# REPO = имя репо (= имя тома/сети-alias/контейнера). Берётся из АРГУМЕНТА продукта, НЕ из
# basename(pwd): на хосте кода репо больше нет (истина в томе). Fallback (in-repo dogfood/тест) —
# basename git-toplevel, если аргумент опущен.
REPO=""
REPO_URL=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo-url) REPO_URL="${2:-}"; shift 2 ;;
    --repo-url=*) REPO_URL="${1#--repo-url=}"; shift ;;
    -*) echo "✖ неизвестный флаг: $1" >&2; exit 64 ;;
    *) [ -z "$REPO" ] && REPO="$1"; shift ;;
  esac
done
[ -n "$REPO" ] || REPO=$(basename "$(git rev-parse --show-toplevel 2>/dev/null || pwd)")
[ -n "$REPO" ] || { echo "✖ укажи <repo> (имя репо = имя тома/сети/контейнера)" >&2; exit 64; }

CONTAINER="${REPO}-devbox"
WORKSPACE_VOLUME="${REPO}-workspace"
WORKDIR="/workspaces/${REPO}"
NETWORK="omnifield-gateway"
DRY="${DEVBOX_DRY_RUN:-}"
BOOTSTRAP_IMAGE="${DEVBOX_BOOTSTRAP_IMAGE:-ghcr.io/omnifield/devbox:latest}"
: "${REPO_URL:=https://github.com/omnifield/${REPO}.git}"
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

# docker нужен всегда, КРОМЕ dry-run (там команды только печатаются).
[ -n "$DRY" ] || command -v docker >/dev/null 2>&1 || {
  echo "✖ нужен docker в PATH (containers-only)" >&2; exit 1; }

# docker-исполнитель с dry-run (печатает вместо запуска). Все МУТАЦИИ идут через dk.
# Dry-печать — в STDERR: иначе легитимные `>/dev/null` вызывателей (глушат container/network-ID
# на реальном прогоне) съели бы намеренную команду, и `docker create` не был бы виден в proof.
dk() {
  if [ -n "$DRY" ]; then
    {
      printf 'docker'
      for a in "$@"; do printf ' %s' "$a"; done
      printf '\n'
    } >&2
    return 0
  fi
  docker "$@"
}

# Инспекции (read-only) — в dry-run НЕ трогают docker: возвращают «нет» (печатаем полный up-путь).
exists()        { [ -n "$DRY" ] && return 1; docker inspect "$CONTAINER" >/dev/null 2>&1; }
running()       { [ -n "$DRY" ] && return 1; [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null || echo false)" = "true" ]; }
vol_exists()    { [ -n "$DRY" ] && return 1; docker volume inspect "$WORKSPACE_VOLUME" >/dev/null 2>&1; }
# Том наполнен, если в нём лежит манифест репо (клон состоялся).
vol_populated() {
  [ -n "$DRY" ] && return 1
  docker run --rm -v "$WORKSPACE_VOLUME:$WORKDIR:ro" "$BOOTSTRAP_IMAGE" \
    test -e "$WORKDIR/.devcontainer/devcontainer.json" >/dev/null 2>&1
}

# Эмиттер: прод — node в bootstrap-образе, манифест ИЗ ТОМА (ro); тест — хостовый node из
# host-манифеста. Разбор devcontainer.json (JSONC) + канон-сторож — в devbox-manifest.mjs.
emit() {  # emit create-args   |   emit hook <field>
  if [ -n "${DEVBOX_EMITTER_LOCAL:-}" ]; then
    m="${DEVBOX_LOCAL_MANIFEST:-$PWD/.devcontainer/devcontainer.json}"
    node "$SCRIPT_DIR/devbox-manifest.mjs" "$1" "$m" ${2:+"$2"}
  else
    docker run --rm -v "$WORKSPACE_VOLUME:$WORKDIR:ro" "$BOOTSTRAP_IMAGE" \
      node "$WORKDIR/scripts/devbox-manifest.mjs" "$1" "$WORKDIR/.devcontainer/devcontainer.json" ${2:+"$2"}
  fi
}

ensure_network() {
  if [ -z "$DRY" ] && docker network inspect "$NETWORK" >/dev/null 2>&1; then return 0; fi
  echo "[devbox] docker-сеть $NETWORK (один раз на машину, idempotent)…" >&2
  dk network create "$NETWORK" >/dev/null 2>&1 || true
}

# Наполнение workspace-тома git-клоном (только если тома нет / он пуст) — идемпотентно.
ensure_workspace_volume() {
  if vol_exists && vol_populated; then
    echo "[devbox] том $WORKSPACE_VOLUME наполнен — клон не нужен (идемпотентно)." >&2
    return 0
  fi
  echo "[devbox] том $WORKSPACE_VOLUME пуст/отсутствует — создаю + git-clone $REPO_URL…" >&2
  dk volume create "$WORKSPACE_VOLUME" >/dev/null
  # Клон В ТОМ: creds (gitconfig/gh) из тома omnifield-secrets (ro); bootstrap-образ несёт git.
  dk run --rm \
    -v "$WORKSPACE_VOLUME:$WORKDIR" \
    -v "omnifield-secrets:/home/vscode/.secrets:ro" \
    -e "GIT_CONFIG_GLOBAL=/home/vscode/.secrets/gitconfig" \
    -e "GH_CONFIG_DIR=/home/vscode/.secrets/gh" \
    "$BOOTSTRAP_IMAGE" \
    git clone "$REPO_URL" "$WORKDIR"
  # Свежий named-том root-owned → chown на vscode(1000) (гага пилота brainer 2026-07-24).
  dk run --rm -v "$WORKSPACE_VOLUME:$WORKDIR" "$BOOTSTRAP_IMAGE" \
    chown -R 1000:1000 "$WORKDIR"
}

run_hook() {  # $1=hook-поле $2=strict|soft
  hook=$(emit hook "$1")
  [ -n "$hook" ] || return 0
  echo "[devbox] $1…" >&2
  if [ "$2" = "strict" ]; then
    dk exec "$CONTAINER" sh -c "$hook"
  else
    dk exec "$CONTAINER" sh -c "$hook" || echo "[devbox] $1 не прошёл (мягкий) — продолжаю" >&2
  fi
}

create_container() {
  echo "[devbox] создаю $CONTAINER (том $WORKSPACE_VOLUME → $WORKDIR; инвариант ставит провизионер)…" >&2
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
    -v "${WORKSPACE_VOLUME}:${WORKDIR}" \
    -w "$WORKDIR" \
    "$@" >/dev/null
}

# fail-loud ext4-guard (ЯДРО анти-инцидента DEVOPSER-188): рабочая копия ОБЯЗАНА жить на
# ext4-томе и быть непустой. tmpfs (деградация/гонка) ИЛИ пустой $WORKDIR → НЕ оставляем тихую
# пустышку (корень часового простоя 2026-07-24) — падаем громко + подсказываем recreate.
guard_workspace() {
  if [ -n "$DRY" ]; then
    echo "[devbox] (dry) guard: findmnt -no FSTYPE $WORKDIR → ждём ext4 (не tmpfs) + $WORKDIR/.git непуст." >&2
    return 0
  fi
  fs=$(docker exec "$CONTAINER" sh -c \
    "findmnt -no FSTYPE $WORKDIR 2>/dev/null || stat -f -c %T $WORKDIR 2>/dev/null" | head -n1 || true)
  empty=""
  docker exec "$CONTAINER" test -e "$WORKDIR/.git" >/dev/null 2>&1 || empty=1
  if [ "$fs" = "tmpfs" ] || [ -n "$empty" ]; then
    echo "✖ [devbox] GUARD: $WORKDIR ${empty:+пуст }${fs:+(fstype=$fs)} — тихая пустышка НЕ допускается." >&2
    echo "  Причина: том не наполнен ИЛИ смонтирован как tmpfs (деградация host-bind→tmpfs, инцидент 2026-07-24)." >&2
    echo "  Лечение: scripts/devbox.sh recreate $REPO   # том/данные переживают; пустой — переклонится." >&2
    exit 1
  fi
  echo "[devbox] guard: $WORKDIR на ${fs:-?}, .git на месте — ок." >&2
}

do_up() {
  ensure_network
  if exists; then
    if running; then
      echo "[devbox] $CONTAINER уже поднят — no-op (идемпотентно)." >&2
      guard_workspace
      return 0
    fi
    echo "[devbox] $CONTAINER есть, но остановлен — docker start…" >&2
    dk start "$CONTAINER" >/dev/null
    run_hook postStartCommand soft
    guard_workspace
    return 0
  fi
  ensure_workspace_volume
  create_container
  dk start "$CONTAINER" >/dev/null
  run_hook postCreateCommand strict
  run_hook postStartCommand soft
  guard_workspace
  echo "[devbox] $CONTAINER поднят. Вход: scripts/devbox-session.sh <scope>" >&2
}

do_down() {
  if exists; then
    echo "[devbox] удаляю $CONTAINER (том $WORKSPACE_VOLUME/данные не трогаю — переживут recreate)…" >&2
    dk rm -f "$CONTAINER" >/dev/null
  elif [ -n "$DRY" ]; then
    echo "[devbox] (dry) удалил бы $CONTAINER (том сохраняется)…" >&2
    dk rm -f "$CONTAINER" >/dev/null
  else
    echo "[devbox] $CONTAINER уже нет — no-op." >&2
  fi
}

case "$CMD" in
  up) do_up ;;
  down) do_down ;;
  recreate) do_down; do_up ;;
  *) echo "usage: devbox.sh <up|down|recreate> <repo> [--repo-url URL]" >&2; exit 64 ;;
esac
