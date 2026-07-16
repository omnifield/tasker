#!/usr/bin/env node
// devbox-services.mjs — оркестратор dev-сервисов devbox (briefs/devbox-first-run-dx-design.md A2/A3/A5).
// Zero-deps (только node:*), как init.mjs: образ = тонкая оболочка, менеджер процессов = сам скрипт,
// без supervisord/overmind (канон).
//
//   devbox-services up            — старт всех задекларированных (детач), idempotent; probe G1
//   devbox-services start [svc]    — старт всех / одного (детач)
//   devbox-services stop  [svc]    — стоп всех / одного (по pidfile, kill группы)
//   devbox-services restart [svc]  — стоп+старт
//   devbox-services status [svc]   — таблица: сервис · pid · порт · bind · health
//   devbox-services run   <svc>    — FOREGROUND один сервис (HMR-eyeball), Ctrl-C = стоп
//   devbox-services logs  [svc]    — tail -f лога
//
// Декларация — devbox.services.json в корне репо (см. A1). State — ~/.devbox (home = cattle,
// не workspace → не пачкает диф, не gitignore'ится).

import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, openSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));
const STATE = join(homedir(), ".devbox");
const RUN = join(STATE, "run");
const LOGS = join(STATE, "logs");

const pidFile = (name) => join(RUN, `${name}.pid`);
const logFile = (name) => join(LOGS, `${name}.log`);

function die(msg) {
  console.error(msg);
  process.exit(1);
}

// --- Декларация ------------------------------------------------------------

// Ищем devbox.services.json вверх от cwd (запуск из любой поддиректории репо),
// с фолбэком на корень репо относительно скрипта (scripts/devbox-services.mjs → ../).
function findConfig() {
  const names = ["devbox.services.json"];
  const starts = [process.cwd(), resolve(HERE, "..")];
  for (const start of starts) {
    let dir = start;
    for (;;) {
      for (const n of names) {
        const p = join(dir, n);
        if (existsSync(p)) return p;
      }
      const up = dirname(dir);
      if (up === dir) break;
      dir = up;
    }
  }
  return null;
}

function loadServices() {
  const cfgPath = findConfig();
  if (!cfgPath)
    die(
      "[devbox-services] devbox.services.json не найден (корень репо). Декларация сервисов — см. brief A1.",
    );
  let raw;
  try {
    raw = JSON.parse(readFileSync(cfgPath, "utf8"));
  } catch (e) {
    die(`[devbox-services] devbox.services.json не парсится: ${e.message}`);
  }
  if (!Array.isArray(raw))
    die("[devbox-services] devbox.services.json обязан быть массивом сервисов (см. A1).");
  const repoRoot = dirname(cfgPath);
  for (const s of raw) {
    if (!s.name || !s.command || s.port === undefined || !s.cwd)
      die(
        `[devbox-services] сервис неполон (обяз. name·cwd·command·port): ${JSON.stringify(s)}`,
      );
    validateCommand(s);
  }
  return { repoRoot, services: raw };
}

// G2-валидатор (A1): литеральный ` -- ` перед --host/--port — pnpm/npm глотают его в свой парсер,
// флаг до vite не долетает → сервис слушает 127.0.0.1 (см. инцидент, грабля G2).
function validateCommand(s) {
  const tokens = s.command.split(/\s+/);
  const sep = tokens.indexOf("--");
  if (sep !== -1) {
    const after = tokens.slice(sep + 1).join(" ");
    if (/--(host|port)/.test(after))
      die(
        `[devbox-services] G2 в "${s.name}": литеральный ' -- ' перед --host/--port ` +
          `(command="${s.command}"). Убери ' -- ' — pnpm/npm прокинут --host/--port в vite напрямую.`,
      );
  }
}

// --- Процессы --------------------------------------------------------------

const isAlive = (pid) => {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
};

function runningPid(name) {
  const pf = pidFile(name);
  if (!existsSync(pf)) return null;
  const pid = Number(readFileSync(pf, "utf8").trim());
  if (pid && isAlive(pid)) return pid;
  rmSync(pf, { force: true }); // stale
  return null;
}

function startOne(svc, repoRoot) {
  const existing = runningPid(svc.name);
  if (existing) {
    console.log(`[devbox-services] ${svc.name}: уже поднят (pid ${existing}) — skip.`);
    return existing;
  }
  mkdirSync(RUN, { recursive: true });
  mkdirSync(LOGS, { recursive: true });
  const cwd = resolve(repoRoot, svc.cwd);
  if (!existsSync(cwd))
    die(`[devbox-services] ${svc.name}: cwd не существует: ${cwd}`);
  const fd = openSync(logFile(svc.name), "a");
  // detached → дочерний становится лидером группы; exec заменяет sh самим сервером
  // (pid pidfile'а = реальный процесс, kill -pid валит всю группу).
  const child = spawn("sh", ["-c", `exec ${svc.command}`], {
    cwd,
    detached: true,
    stdio: ["ignore", fd, fd],
    env: process.env,
  });
  child.unref();
  writeFileSync(pidFile(svc.name), String(child.pid));
  console.log(`[devbox-services] ${svc.name}: старт (pid ${child.pid}) → ${logFile(svc.name)}`);
  return child.pid;
}

function stopOne(name) {
  const pid = runningPid(name);
  if (!pid) {
    console.log(`[devbox-services] ${name}: не запущен.`);
    return;
  }
  try {
    process.kill(-pid, "SIGTERM"); // группа
  } catch {
    try {
      process.kill(pid, "SIGTERM");
    } catch {
      /* уже мёртв */
    }
  }
  rmSync(pidFile(name), { force: true });
  console.log(`[devbox-services] ${name}: остановлен (pid ${pid}).`);
}

// --- Probe (G1) + health ---------------------------------------------------

// Разбор `ss -ltnH`: возвращает 'all' | 'loopback' | null для порта.
function probeBind(port) {
  const res = spawnSync("ss", ["-ltnH"], { encoding: "utf8" });
  if (res.error || res.status !== 0) return "unknown"; // ss нет — не блокируем, но честно
  let loopbackOnly = null;
  for (const line of res.stdout.split("\n")) {
    const cols = line.trim().split(/\s+/);
    // LISTEN Recv-Q Send-Q Local Peer ...  → Local = cols[3]
    const local = cols[3];
    if (!local) continue;
    const m = local.match(/:(\d+)$/);
    if (!m || Number(m[1]) !== port) continue;
    const addr = local.slice(0, local.lastIndexOf(":"));
    const wildcard = addr === "0.0.0.0" || addr === "*" || addr === "[::]" || addr === "::";
    if (wildcard) return "all";
    loopbackOnly = "loopback";
  }
  return loopbackOnly; // 'loopback' если были только loopback-слушатели, иначе null
}

async function health(url) {
  if (!url) return "n/a";
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), 2000);
    const res = await fetch(url, { signal: ctrl.signal });
    clearTimeout(t);
    return res.ok ? "ok" : `http ${res.status}`;
  } catch {
    return "fail";
  }
}

// После старта: ждём слушателя до timeout, ловим G1 (loopback-bind) громко.
function probeStartup(svc) {
  const t0 = Date.now();
  // Холодный vite/webpack-билд может превысить фикс-15s → ложный WARN. Дефолт 30s,
  // настраиваемый env'ом (кривой/пустой ввод → дефолт, не 0). Семантику G1-die/детача НЕ трогаем.
  // Приоритет: per-service (devbox.services.json probeTimeoutMs) → env → дефолт.
  // Cold `go run` (докачка модулей + компиляция) легко >30s → щедрый дефолт + per-service override.
  const svcT = Number.parseInt(String(svc.probeTimeoutMs ?? ""), 10);
  const envT = Number.parseInt(process.env.DEVBOX_PROBE_TIMEOUT_MS ?? "", 10);
  const timeoutMs =
    (Number.isFinite(svcT) && svcT > 0 && svcT) ||
    (Number.isFinite(envT) && envT > 0 && envT) ||
    90000;
  for (;;) {
    const bind = probeBind(svc.port);
    if (bind === "all") return { ok: true };
    if (bind === "loopback") {
      stopOne(svc.name);
      die(
        `[devbox-services] G1 в "${svc.name}": слушает 127.0.0.1:${svc.port}, НЕ 0.0.0.0. ` +
          `Сосед по docker-сети (gateway) не достучится до loopback → 502. ` +
          `command обязан bind 0.0.0.0 (vite: --host 0.0.0.0; uvicorn: --host 0.0.0.0).`,
      );
    }
    if (bind === "unknown") return { ok: null, note: "ss недоступен — G1-probe пропущен" };
    if (!isAlive(runningPid(svc.name) ?? -1)) {
      return { ok: false, note: "процесс упал на старте (см. logs)" };
    }
    if (Date.now() - t0 > timeoutMs) {
      return { ok: false, note: `порт ${svc.port} не слушается за ${timeoutMs / 1000}s` };
    }
    // короткий сон между poll'ами (zero-dep, не жрёт CPU)
    spawnSync("sleep", ["0.4"]);
  }
}

// --- Команды ---------------------------------------------------------------

function resolveTargets(services, svcArg) {
  if (!svcArg) return services;
  const one = services.find((s) => s.name === svcArg);
  if (!one) die(`[devbox-services] сервис не найден: ${svcArg}`);
  return [one];
}

async function cmdUp(repoRoot, targets) {
  for (const s of targets) startOne(s, repoRoot);
  // probe детач-стартов (последовательно, но каждый — короткий poll)
  for (const s of targets) {
    if (!runningPid(s.name)) continue;
    const r = probeStartup(s);
    if (r.ok === true) console.log(`[devbox-services] ${s.name}: bind 0.0.0.0:${s.port} ✓`);
    else if (r.note) console.warn(`[devbox-services] ${s.name}: ⚠ ${r.note}`);
  }
  console.log("[devbox-services] up завершён (сервисы детачнуты, управление возвращено).");
}

async function cmdStatus(targets) {
  const rows = [];
  for (const s of targets) {
    const pid = runningPid(s.name);
    const bind = pid ? probeBind(s.port) : null;
    const bindStr = bind === "all" ? "0.0.0.0" : bind === "loopback" ? "127.0.0.1⚠" : "-";
    const h = pid ? await health(s.healthUrl) : "-";
    rows.push([s.name, pid ? String(pid) : "-", String(s.port), bindStr, h]);
  }
  const head = ["SERVICE", "PID", "PORT", "BIND", "HEALTH"];
  const widths = head.map((h, i) =>
    Math.max(h.length, ...rows.map((r) => r[i].length)),
  );
  const fmt = (r) => r.map((c, i) => c.padEnd(widths[i])).join("  ");
  console.log(fmt(head));
  for (const r of rows) console.log(fmt(r));
}

function cmdRun(svc) {
  // foreground: один сервис, живые логи, Ctrl-C = стоп.
  console.log(`[devbox-services] run ${svc.name} (foreground, Ctrl-C = стоп)…`);
  const { repoRoot } = loadServices();
  const cwd = resolve(repoRoot, svc.cwd);
  const child = spawn("sh", ["-c", `exec ${svc.command}`], { cwd, stdio: "inherit" });
  const forward = (sig) => () => child.kill(sig);
  process.on("SIGINT", forward("SIGINT"));
  process.on("SIGTERM", forward("SIGTERM"));
  child.on("exit", (code) => process.exit(code ?? 0));
}

function cmdLogs(targets) {
  const files = targets.map((s) => logFile(s.name)).filter(existsSync);
  if (!files.length) die("[devbox-services] логов пока нет (сервис не стартовал?).");
  const child = spawn("tail", ["-n", "100", "-f", ...files], { stdio: "inherit" });
  child.on("exit", (code) => process.exit(code ?? 0));
}

async function main() {
  const [cmd, svcArg] = process.argv.slice(2);
  if (!cmd || ["-h", "--help", "help"].includes(cmd)) {
    console.log(
      "devbox-services <up|start|stop|restart|status|run|logs> [service]\n" +
        "  up/start [svc]  старт (детач)   stop [svc]     стоп\n" +
        "  restart [svc]   рестарт          status [svc]   таблица\n" +
        "  run <svc>       foreground       logs [svc]     tail -f",
    );
    return;
  }

  // run/logs грузят конфиг сами по мере надобности
  if (cmd === "run") {
    const { services } = loadServices();
    if (!svcArg) die("[devbox-services] run требует имя сервиса.");
    cmdRun(resolveTargets(services, svcArg)[0]);
    return;
  }

  const { repoRoot, services } = loadServices();
  const targets = resolveTargets(services, svcArg);

  switch (cmd) {
    case "up":
    case "start":
      await cmdUp(repoRoot, targets);
      break;
    case "stop":
      for (const s of targets) stopOne(s.name);
      break;
    case "restart":
      for (const s of targets) stopOne(s.name);
      await cmdUp(repoRoot, targets);
      break;
    case "status":
      await cmdStatus(targets);
      break;
    case "logs":
      cmdLogs(targets);
      break;
    default:
      die(`[devbox-services] неизвестная команда: ${cmd}`);
  }
}

main();
