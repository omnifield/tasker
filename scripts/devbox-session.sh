#!/usr/bin/env sh
# devbox-session.sh — штатный вход в devbox репо одной командой (brief B6 + gateway-network step 4).
#
#   scripts/devbox-session.sh [scope]     # из корня репо; scope по умолчанию — main
#
# Тонкая session-entry (СКЕЛЕТ = чистая инфра): резолвит devbox-контейнер репо → docker exec -it
# с OMNIFIELD_SCOPE (identity/валидацию/git-gate — scope-identity/marker/scope-resolve — заводит сам
# scope на SessionStart, per-product), дёргает idempotent `devbox-services up` (A4 safety-net) и
# запускает claude. Роль/identity зон — за scope-хуками; УНИВЕРСАЛЬНАЯ политика model-pin
# (owner-скоуп → opus, main → своя модель) живёт ЗДЕСЬ единственным местом (ретайр claude-scope.ps1,
# Шаг 3): скелет-меха одинакова для любого продукта, скоупы — per-product.
#
# Канон containers-only: на ХОСТЕ — только docker (node/git может не быть) → launcher завязан ТОЛЬКО
# на `docker`. Контейнер НЕ создаёт (это VS Code «Reopen in Container» через .devcontainer/, либо
# workstation-диспетчер `oa` — follow-up); зато гарантирует gateway-сеть через `docker network
# connect` (single-origin: контейнер в сети omnifield-gateway под alias=имя-репо, наружу — ничего,
# только gateway :8080).
set -eu

SCOPE="${1:-main}"
[ "$#" -gt 0 ] && shift || true   # остаток аргументов уходит в claude
NETWORK="omnifield-gateway"

command -v docker >/dev/null 2>&1 || { echo "✖ нужен docker в PATH"; exit 1; }

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
REPO=$(basename "$REPO_ROOT")

# --- резолвим контейнер репо ----------------------------------------------
# Канон-конвенция имени `${repo}-devbox` = КОНТРАКТ (её ставит Шаг-2 провижинер `devbox up`);
# имена НЕ хардкодятся в скрипте — вычисляются из basename репо. Fallback — метка VS Code
# devcontainer (Reopen in Container кладёт свою). Каталог продуктов — registry/products.md.
CONTAINER="${REPO}-devbox"
if ! docker inspect "$CONTAINER" >/dev/null 2>&1; then
  CONTAINER=$(docker ps -aq --filter "label=devcontainer.local_folder=$REPO_ROOT" | head -n1 || true)
fi
if [ -z "$CONTAINER" ]; then
  echo "✖ devbox-контейнер для '$REPO' не найден."
  echo "  Подними его одним из путей входа:"
  echo "    • headless: scripts/devbox.sh up   (провижн из .devcontainer/devcontainer.json, по канону)"
  echo "    • VS Code:  «Reopen in Container» (.devcontainer/devcontainer.json)"
  exit 1
fi

# --- поднять если стоит ----------------------------------------------------
if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null || echo false)" != "true" ]; then
  echo "[devbox-session] $CONTAINER остановлен — docker start…"
  docker start "$CONTAINER" >/dev/null
fi

# --- гарантируем gateway-сеть + alias (single-origin) ----------------------
docker network inspect "$NETWORK" >/dev/null 2>&1 || {
  echo "[devbox-session] создаю внешнюю docker-сеть $NETWORK (один раз на машину)…"
  docker network create "$NETWORK" >/dev/null
}
# idempotent: если уже подключён — docker ругнётся, глотаем.
docker network connect --alias "$REPO" "$NETWORK" "$CONTAINER" 2>/dev/null || true

# --- safety-net autostart (idempotent no-op если подняты) ------------------
docker exec "$CONTAINER" sh -c 'node scripts/devbox-services.mjs up 2>/dev/null || true' || true

# --- model-pin (универсальная политика, единственное место; ретайр .ps1) ---
# owner-скоуп (не main) → --model opus; main → своя модель (не навязываем). Явный --model
# юзера НЕ перетираем. Решение по scope — node на хосте не нужен (containers-only).
has_model=""
for a in "$@"; do
  case "$a" in --model | --model=*) has_model=1 ;; esac
done
if [ "$SCOPE" != "main" ] && [ -z "$has_model" ]; then
  echo "[devbox-session] owner-model: opus (scope=$SCOPE)"
  set -- --model opus "$@"
fi

# --- вход агентом ----------------------------------------------------------
echo "[devbox-session] вход: repo=$REPO, scope=$SCOPE"
exec docker exec -it -e "OMNIFIELD_SCOPE=$SCOPE" -w "/workspaces/$REPO" "$CONTAINER" claude "$@"
