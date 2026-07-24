#!/usr/bin/env sh
# recover.sh — ordered host-driven bring-up девбоксов ПОСЛЕ готовности WSL/докера
# (DEVOPSER-188, ADR-16). Интерим-подъём после ребута хоста.
#
#   scripts/recover.sh <repo> [<repo> …]   # ждёт docker-демон → devbox.sh up для каждого по порядку
#
# ЗАЧЕМ. `--restart unless-stopped` поднимал девбоксы РАНЬШЕ готовности WSL — корень tmpfs-гонки
# host-bind (пустой /workspaces как tmpfs → RAM-крах, инцидент 2026-07-24). В volume-модели
# (DEVOPSER-188) том всегда ext4 → авто-рестарт БЕЗОПАСЕН; recover.sh не заменяет его, а даёт
# ДЕТЕРМИНИРОВАННЫЙ ordered bring-up: сперва дождаться docker-демона, затем поднять продукты по
# порядку одной идемпотентной командой. fail-loud guard в devbox.sh (tmpfs/пустой том) — вторая
# линия; recover.sh — детерминизм порядка. Минимальный не-костыльный вариант (DEVOPSER-188 п.7):
# ноль per-репо-данных в скрипте — список продуктов машины передаёт вызыватель (systemd-unit /
# ручной прогон), tool остаётся generic (как devbox.sh: repo = аргумент).
set -eu

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

[ "$#" -gt 0 ] || { echo "usage: recover.sh <repo> [<repo> …]" >&2; exit 64; }
command -v docker >/dev/null 2>&1 || { echo "✖ нужен docker в PATH (containers-only)" >&2; exit 1; }

echo "[recover] жду готовности docker-демона…" >&2
until docker info >/dev/null 2>&1; do sleep 2; done
echo "[recover] docker готов — ordered bring-up: $*" >&2

for repo in "$@"; do
  echo "[recover] → devbox.sh up $repo" >&2
  "$SCRIPT_DIR/devbox.sh" up "$repo"
done
echo "[recover] готово." >&2
