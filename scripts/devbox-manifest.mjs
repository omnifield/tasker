#!/usr/bin/env node
// devbox-manifest.mjs — переводит канон-манифест `.devcontainer/devcontainer.json` в
// аргументы `docker create` для headless-провижинера devbox.sh (Foundation Шаг 2,
// briefs/devbox-provision-lifecycle.md). Zero-deps, только node:*.
//
// Единый источник = devcontainer.json (VS Code-путь и headless-провижн не разъезжаются).
// Разделение ответственности:
//   • канон-ИНВАРИАНТ добавляет сам провизионер (network=omnifield-gateway + alias=repo,
//     --restart unless-stopped, единственный bind своего репо в /workspaces/<repo>) — НЕ из
//     манифеста, чтобы инвариант держался для любого devbox;
//   • ПРОДУКТ-переменное тянется из манифеста (image, containerEnv, extra-volumes, hooks);
//   • канон-СТОРОЖ (loud-fail, не тихий strip — дефект чинится в манифесте):
//       – host-порт в runArgs (-p/--publish/-P/--expose) → fail (single-origin: наружу
//         только gateway :8080);
//       – god-mount (mount target=/workspaces) → fail (свой репо монтирует провизионер);
//       – --network/--network-alias в runArgs → fail (канон-сеть ставит провизионер, не манифест).
//
//   node devbox-manifest.mjs create-args <manifest>          → docker-create аргументы (токен/строка)
//   node devbox-manifest.mjs hook <manifest> <hook-поле>     → строка хука (пусто, если нет)
//         hook-поле ∈ { postCreateCommand, postStartCommand }

import { readFileSync } from "node:fs";

// Контейнер без long-running CMD сразу выходит — держим живым (форма VS Code overrideCommand).
const KEEPALIVE = ["sh", "-c", "while sleep 2073600; do :; done"];

function fail(msg) {
  process.stderr.write(`[devbox-manifest] КАНОН-ДЕФЕКТ манифеста: ${msg}\n`);
  process.exit(2);
}

function parseManifest(path) {
  let raw;
  try {
    raw = readFileSync(path, "utf8");
  } catch {
    fail(`манифест не читается: ${path} (нет .devcontainer/devcontainer.json?)`);
  }
  // devcontainer.json — JSONC: срезаем /* */ и // комментарии + trailing-запятые.
  const stripped = raw
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/(^|[^:])\/\/.*$/gm, "$1")
    .replace(/,(\s*[}\]])/g, "$1");
  try {
    return JSON.parse(stripped);
  } catch (e) {
    fail(`невалидный JSON(C): ${e.message}`);
  }
}

function createArgs(m) {
  const out = [];
  // containerEnv → -e KEY=VALUE (наследуется и docker exec'ом хуков/сессии).
  for (const [k, v] of Object.entries(m.containerEnv ?? {})) out.push("-e", `${k}=${v}`);
  // mounts → --mount <str>; god-mount guard.
  for (const mt of m.mounts ?? []) {
    if (/(^|,)target=\/workspaces(\/|,|$)/.test(mt))
      fail(`mount тянет god-mount /workspaces («${mt}») — свой репо монтирует провизионер`);
    out.push("--mount", mt);
  }
  // runArgs passthrough с канон-сторожем.
  for (const a of m.runArgs ?? []) {
    if (/^(-p|--publish|-P|--expose)(=|$)/.test(a))
      fail(`runArgs публикует host-порт («${a}») — single-origin, наружу только gateway :8080`);
    if (/^--network(-alias)?=/.test(a)) continue; // канон-сеть/alias добавит провизионер
    if (/^--network(-alias)?$/.test(a))
      fail(`«${a}» в bare-форме — используй =-форму; канон-сеть ставит провизионер`);
    out.push(a);
  }
  if (!m.image) fail("в манифесте нет поля image");
  out.push(m.image, ...KEEPALIVE);
  return out;
}

function main() {
  const [sub, manifestPath, field] = process.argv.slice(2);
  if (!sub || !manifestPath) {
    process.stderr.write("usage: devbox-manifest.mjs <create-args|hook> <manifest> [hook-field]\n");
    process.exit(64);
  }
  const m = parseManifest(manifestPath);
  if (sub === "create-args") {
    process.stdout.write(`${createArgs(m).join("\n")}\n`);
  } else if (sub === "hook") {
    if (field !== "postCreateCommand" && field !== "postStartCommand")
      fail(`неизвестное hook-поле: ${field}`);
    const v = m[field];
    if (v) process.stdout.write(String(v));
  } else {
    process.stderr.write(`unknown subcommand: ${sub}\n`);
    process.exit(64);
  }
}

main();
