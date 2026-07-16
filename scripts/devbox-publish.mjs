#!/usr/bin/env node
// devbox-publish.mjs — публикация product-манифеста в общий registry-volume
// (Foundation Шаг 5 §A, briefs/feedback-hub-core-as-hub-under-isolation.md).
//
// Discovery = publish-volume (НЕ exec-Канал): каждый продукт-devbox на старте кладёт
// свой omnifield.yaml в omnifield-registry/<name>.yaml (named volume). hub-core глобит
// оттуда *.yaml (ro) вместо fs-скана сиблингов — реестр НЕ зависит от up-состояния,
// маршрут не моргает (last-published-wins). Декларатив, не скрейп из процесса.
//
// Zero-deps (только node:*), как остальной devbox-набор. Запускается из postStartCommand
// РЯДОМ с `devbox-services up` (Шаг 4) — оба на старте devbox, но разные концерны:
// services = жизненный цикл dev-сервисов, publish = публикация манифеста в реестр.
//
//   node scripts/devbox-publish.mjs
//
// <name> = basename репо = --network-alias (single-origin join-key, канонический ключ —
// liaison-inc1-manifest-boundary). Манифеста в корне репо нет → loud-warn + no-op (exit 0):
// продукт без манифеста просто остаётся вне двери :8080 — это норма, декларация манифеста
// = зона owner'а продукта (Шаг 5.2), не отказ devbox.

import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));
// scripts/devbox-publish.mjs → корень репо = scripts/.. (workspace-root, где лежит манифест).
const REPO_ROOT = resolve(HERE, "..");
const NAME = basename(REPO_ROOT);
const MANIFEST = join(REPO_ROOT, "omnifield.yaml");
// Общий named volume, куда пишут ВСЕ продукт-devbox'ы и откуда hub-core глобит *.yaml (ro).
// Путь-монтирования = mount target из .devcontainer/devcontainer.json (единый источник);
// env-override — только для тестов / нестандартной схемы монтирования.
const REGISTRY_DIR = process.env.OMNIFIELD_REGISTRY_DIR || "/omnifield-registry";

function main() {
  if (!existsSync(MANIFEST)) {
    console.warn(
      `[devbox-publish] ⚠ ${NAME}: нет omnifield.yaml в корне репо (${MANIFEST}) — ` +
        `публиковать нечего, продукт останется вне двери :8080. No-op. ` +
        `Декларация манифеста = зона owner'а продукта (contract @omnifield/contract-manifest, Шаг 5.2).`,
    );
    return; // no-op, exit 0 — отсутствие манифеста это норма, не отказ
  }
  const dest = join(REGISTRY_DIR, `${NAME}.yaml`);
  try {
    mkdirSync(REGISTRY_DIR, { recursive: true });
    // Копия байт-в-байт: валидация (Zod) и loud-warn по невалидному — концерн hub-core,
    // devbox только публикует product-owned манифест как есть.
    copyFileSync(MANIFEST, dest);
  } catch (e) {
    // registry-volume не смонтирован / не writable — реальный дефект провижна (mount/права),
    // громко на stderr. Вызывающий чейнит `;` (не `&&`) — dev-сервисы поднимутся независимо.
    console.error(
      `[devbox-publish] ✖ ${NAME}: не удалось опубликовать манифест в ${dest}: ${e.message}. ` +
        `Смонтирован ли omnifield-registry (rw) и chown'нут на vscode? (.devcontainer mounts + postCreate).`,
    );
    process.exitCode = 1;
    return;
  }
  console.log(`[devbox-publish] ${NAME}: omnifield.yaml → ${dest} ✓`);
}

main();
