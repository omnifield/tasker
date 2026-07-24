#!/usr/bin/env node
// git-flow.mjs — agent-agnostic git-ИНСТРУМЕНТ (DEVOPSER-106). Managed-скрипт (как
// devbox-services.mjs): вендорится в каждый репо, ЧИТАЕТ ВЕНДОРЕННЫЙ git-flow.json (managed-файл,
// DEVOPSER-113 — language-agnostic, любой стек, ноль npm) и делает полный луп git без ручных
// команд. Zero-deps (node:* + шелл git/gh).
//
//   git-flow start <type>/<slug>   — ветка ОТ origin/main (свежий fetch; урок PR#26: не от грязного
//                                    local); имя валидируется против defaults.branchNaming.
//   git-flow commit <msg>          — коммит; defaults.commitConvention валидируется.
//   git-flow push                  — push текущей ветки в origin.
//   git-flow pr [--title T --body B] — открыть PR через gh (--base main).
//   git-flow land                  — frame.prRequired: требует открытый PR; включает AUTO-MERGE
//                                    (gh pr merge --auto по defaults.merge, --delete-branch) и
//                                    возвращается СРАЗУ — GitHub домержит серверно по зелёным
//                                    required-checks ruleset'а (DEVOPSER-157). Локально НЕ ждёт и
//                                    НЕ синкает main — освежает его start (fetch origin/main) или
//                                    явный git-flow sync.
//   git-flow sync                  — локальный main = origin/main.
//   (любая) --dry-run              — печатает мутации (git/gh write), не выполняет.
//
// Политики — ВСЕ из пресета (branchNaming / commitConvention / merge / requiredChecks;
// frame.mainProtected / frame.prRequired enforce). Хардкода флоу нет.
//
// ⚠️ AGENT-AGNOSTIC: инструмент про «кого» НЕ знает — просто выполняет операции. Кто вызывает и
//    кому что можно = концерн ПОТРЕБИТЕЛЯ (не devopser, не инструмент). Ноль owner/ролей/прав/gate.

import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const HERE = dirname(fileURLToPath(import.meta.url));

export function die(msg) {
  console.error(msg);
  process.exit(1);
}

// --- Резолв git-flow пресета (DEVOPSER-113: вендоренный, language-agnostic) -----
// Читаем ЛОКАЛЬНЫЙ вендоренный git-flow.json (managed-файл в корне репо — есть в ЛЮБОМ стеке,
// ноль node_modules; go-primary/polyglot тоже). Идём вверх от cwd И от расположения скрипта
// (робастно для vendored scripts/ и эталона files/). Fallback — legacy npm (@omnifield/git-preset
// ретайрен, но для транзишна).
export function resolvePreset(startDir = process.cwd()) {
  for (const base of [startDir, HERE]) {
    let dir = base;
    for (;;) {
      const p = join(dir, "git-flow.json");
      if (existsSync(p)) return JSON.parse(readFileSync(p, "utf8"));
      const up = dirname(dir);
      if (up === dir) break;
      dir = up;
    }
  }
  const legacy = join(startDir, "node_modules/@omnifield/git-preset/git-flow.json");
  if (existsSync(legacy)) return JSON.parse(readFileSync(legacy, "utf8"));
  die("[git-flow] git-flow.json не найден (вендоренный пресет). Синк: node init.mjs .");
}

// --- Политики пресета (чистые функции — бросают Error, main ловит) -------------

export function validateBranchName(name, pattern) {
  if (!name) throw new Error("start требует <type>/<slug>");
  if (!new RegExp(pattern).test(name))
    throw new Error(`имя ветки "${name}" не по branchNaming ${pattern} (напр. feat/my-slug).`);
  return name;
}

export function validateCommitMessage(msg, convention) {
  if (!msg) throw new Error("commit требует сообщение");
  if (convention === "conventional") {
    // сабж СО СТРОЧНОЙ — Latin ИЛИ кириллица (`: [a-zа-яё]`) — ЗЕРКАЛО CI-гейта
    // .github/workflows/pr-title.yml subjectPattern `^[a-zа-яё].+$` (DEVOPSER-130). НЕ ASCII-only
    // `^[a-z]` — тот резал русские сабжи (живой регресс, #53 «приземлить»); НЕ `\p{Ll}` — amannn
    // гоняет regex без `u`-флага. Иначе `feat: Add x` проходил локальный commit, но падал на
    // pr-title при land — сюрприз на самом дорогом шаге. PAIRED RULE: правишь тут — правь и
    // pr-title.yml (типы уже совпадают: те же 11, вкл. revert). Типы branchNaming (9, без
    // style/revert) намеренно уже — ветка ≠ сабж коммита.
    const re =
      /^(feat|fix|chore|docs|refactor|test|perf|build|ci|style|revert)(\([\w./-]+\))?!?: [a-zа-яё].+/;
    if (!re.test(msg)) throw new Error(`коммит "${msg}" не conventional (type(scope): описание).`);
  }
  return msg;
}

// frame.mainProtected: прямой коммит/пуш в main запрещён — работай на ветке.
export function assertNotMain(branch, frame) {
  if (frame?.mainProtected && branch === "main")
    throw new Error(
      "frame.mainProtected: прямой коммит/пуш в main запрещён — работай на ветке (git-flow start).",
    );
  return branch;
}

export function mergeFlag(merge) {
  const map = { squash: "--squash", merge: "--merge", rebase: "--rebase" };
  if (!map[merge]) throw new Error(`defaults.merge неизвестен: ${merge} (squash|merge|rebase).`);
  return map[merge];
}

// --- Rulesets-материализация (DEVOPSER-110): git-пресет → GitHub-rulesets ------
// Единый источник enforcement = пресет (замещает ручные rulesets). Чистые функции —
// генерация desired-спеки + drift-диф; apply/check шелит gh api (мок в тестах).

const RULESET_NAME = "omnifield-git-flow";

// git-пресет (frame+defaults) + required-checks → desired ruleset-спека (GitHub rulesets API).
// frame.mainProtected → защита ветки по умолчанию (без удаления/force-push); frame.prRequired →
// мерж только через PR; requiredChecks → required status checks. Ноль actor/ролей.
export function buildRulesetSpec(preset, checks) {
  const rules = [];
  if (preset.frame?.mainProtected) {
    rules.push({ type: "deletion" });
    rules.push({ type: "non_fast_forward" });
  }
  if (preset.frame?.prRequired)
    // GitHub Rulesets API требует ПОЛНЫЙ набор pull_request-параметров (иначе 422; DEVOPSER-116).
    // Дефолты сохраняют смысл frame.prRequired = мерж только через PR, без обязательных ревью.
    // required_approving_review_count = 0 ОБЯЗАТЕЛЬНО (DEVOPSER-157): автор PR не может сам его
    // апрувнуть (GitHub self-approve запрещён), а флоу одиночный — потребуй ≥1 ревью, и auto-merge
    // навсегда застрянет без апрувера. Гейт качества = required-checks, не человеко-ревью.
    rules.push({
      type: "pull_request",
      parameters: {
        required_approving_review_count: 0,
        dismiss_stale_reviews_on_push: false,
        require_code_owner_review: false,
        require_last_push_approval: false,
        required_review_thread_resolution: false,
      },
    });
  if (checks.length)
    rules.push({
      type: "required_status_checks",
      parameters: {
        // strict OFF (DEVOPSER-157): не требуем «ветка up-to-date с base». Со strict auto-merge
        // залипал бы, требуя ребейз на свежий main всякий раз, как main уезжал под зелёными checks —
        // фрикция одиночного/агентного флоу. Мерж по зелёным required-checks, без up-to-date-гейта.
        strict_required_status_checks_policy: false,
        required_status_checks: checks.map((c) => ({ context: c })),
      },
    });
  return {
    name: RULESET_NAME,
    target: "branch",
    enforcement: "active",
    conditions: { ref_name: { include: ["~DEFAULT_BRANCH"], exclude: [] } },
    rules,
  };
}

// Каноничный срез ruleset'а для сравнения (GitHub добавляет id/_links/… — игнорируем).
function normalizeRuleset(rs) {
  const rules = rs?.rules ?? [];
  const rsc = rules.find((r) => r.type === "required_status_checks");
  const checks = rsc?.parameters?.required_status_checks ?? [];
  return {
    enforcement: rs?.enforcement,
    target: rs?.target,
    include: rs?.conditions?.ref_name?.include ?? [],
    ruleTypes: rules.map((r) => r.type).sort(),
    checks: checks.map((c) => c.context).sort(),
    // strict в сравнении (DEVOPSER-157): дрейф ловит ручной strict=true против пресетного OFF.
    strict: rsc?.parameters?.strict_required_status_checks_policy ?? null,
  };
}

// Дрейф текущего ruleset против desired (пресет) → список расхождений ([] = чисто).
export function diffRulesets(current, desired) {
  if (!current) return [`ruleset ${RULESET_NAME} отсутствует (не материализован)`];
  const a = normalizeRuleset(current);
  const b = normalizeRuleset(desired);
  const drift = [];
  const cmp = (field, x, y) => {
    if (JSON.stringify(x) !== JSON.stringify(y))
      drift.push(`${field}: ${JSON.stringify(x)} ≠ ${JSON.stringify(y)}`);
  };
  cmp("enforcement", a.enforcement, b.enforcement);
  cmp("target", a.target, b.target);
  cmp("ref_name.include", a.include, b.include);
  cmp("rules", a.ruleTypes, b.ruleTypes);
  cmp("required_checks", a.checks, b.checks);
  cmp("strict", a.strict, b.strict);
  return drift;
}

// --- Repo-settings-материализация (DEVOPSER-157): пресет → настройки репо -------
// Auto-merge требует repo-level настроек, которых ruleset не покрывает: squash-only (метод мержа)
// + allow_auto_merge (иначе `gh pr merge --auto` падает) + delete_branch_on_merge. Единый источник —
// пресет; материализуются тем же `rulesets --apply` (PATCH repos/{nwo}), дрейф — тем же check.

// Пресет → GitHub PATCH-body. Методы мержа ДЕРАЙВЯТСЯ из defaults.merge (единый источник — «squash»
// = allow_squash_merge только; невозможен второй источник правды merge≠settings); auto-merge +
// delete-branch — из defaults.repoSettings (не выводимы из merge).
export function buildRepoSettings(preset) {
  const merge = preset.defaults?.merge;
  const rs = preset.defaults?.repoSettings ?? {};
  return {
    allow_squash_merge: merge === "squash",
    allow_merge_commit: merge === "merge",
    allow_rebase_merge: merge === "rebase",
    allow_auto_merge: rs.autoMerge === true,
    delete_branch_on_merge: rs.deleteBranchOnMerge === true,
  };
}

// Дрейф repo-settings: desired (пресет) vs actual (gh api repos/{nwo}) → расхождения ([] = чисто).
export function diffRepoSettings(current, desired) {
  const drift = [];
  for (const k of Object.keys(desired)) {
    const have = current?.[k] ?? null;
    if (have !== desired[k])
      drift.push(`repo.${k}: ${JSON.stringify(have)} ≠ ${JSON.stringify(desired[k])}`);
  }
  return drift;
}

// --- Executor: тонкая обёртка git/gh (инъектируется — тесты подсовывают мок) ---

export function realExec() {
  const call =
    (bin) =>
    (args, opts = {}) => {
      const r = spawnSync(bin, args, { encoding: "utf8", ...opts });
      return { code: r.status ?? 1, out: r.stdout ?? "", err: r.stderr ?? "" };
    };
  return {
    git: call("git"),
    gh: call("gh"),
    log: (m) => console.log(m),
  };
}

// Мутация (write git/gh): --dry-run печатает, не выполняет; ненулевой код → Error. Опц. input —
// тело запроса (stdin, для gh api --input -).
function mutate(exec, dry, kind, args, input) {
  if (dry) {
    exec.log(`[dry-run] ${kind} ${args.join(" ")}`);
    return { code: 0, out: "", err: "" };
  }
  const r = input === undefined ? exec[kind](args) : exec[kind](args, { input });
  if (r.code !== 0)
    throw new Error(`${kind} ${args.join(" ")} → ${(r.err || r.out || `code ${r.code}`).trim()}`);
  return r;
}

// Чтение (read): ненулевой код → Error с контекстом. Прокидываем stderr git/gh наружу
// (DEVOPSER-114 #3: 403/auth-ошибки видны, а не схлопнуты в "code 1").
function read(exec, kind, args, what) {
  const r = exec[kind](args);
  if (r.code !== 0)
    throw new Error(
      `${what}: ${kind} ${args.join(" ")} → ${(r.err || r.out || `code ${r.code}`).trim()}`,
    );
  return r.out.trim();
}

const currentBranch = (exec) => read(exec, "git", ["rev-parse", "--abbrev-ref", "HEAD"], "ветка");
const repoRoot = (exec) => {
  const r = exec.git(["rev-parse", "--show-toplevel"]);
  return r.code === 0 ? r.out.trim() : process.cwd();
};

// --- Субкоманды ---------------------------------------------------------------

function start(exec, preset, name, { dry }) {
  validateBranchName(name, preset.defaults.branchNaming);
  // ветка ОТ origin/main — свежий fetch, не от грязного local (урок PR#26).
  mutate(exec, dry, "git", ["fetch", "origin", "main"]);
  mutate(exec, dry, "git", ["checkout", "-b", name, "origin/main"]);
  exec.log(`[git-flow] ветка ${name} от origin/main.`);
}

function commit(exec, preset, msg, { dry }) {
  assertNotMain(currentBranch(exec), preset.frame);
  validateCommitMessage(msg, preset.defaults.commitConvention);
  mutate(exec, dry, "git", ["commit", "-m", msg]);
  exec.log("[git-flow] коммит создан.");
}

function push(exec, preset, { dry }) {
  const branch = assertNotMain(currentBranch(exec), preset.frame);
  mutate(exec, dry, "git", ["push", "-u", "origin", branch]);
  exec.log(`[git-flow] ${branch} → origin.`);
}

// Сабжекты коммитов ветки (origin/<base>..HEAD, старые→новые). [] если пусто/ошибка — pr НЕ
// падает на этом (робастно; инвариант ниже всё равно даёт непустые title/body).
function branchCommits(exec, base) {
  const r = exec.git(["log", "--reverse", "--format=%s", `origin/${base}..HEAD`]);
  if (r.code !== 0) return [];
  return r.out
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
}

// Инвариант (DEVOPSER-129): pr ВСЕГДА порождает валидный non-interactive gh — title И body
// удовлетворены (иначе `gh pr create` в non-interactive требует тело → падает).
//  • ноль флагов          → `--fill` (gh выводит title+body из коммитов; чисто non-interactive).
//  • иначе                → явные `--title`/`--body`; недостающее выводим из коммитов ветки,
//                           фолбэк = сам title (затем branch). `--fill` НЕ мешаем с явным
//                           `--title` (gh их не совмещает).
// commits — сабжекты ветки (пустой массив ок); branch — фолбэк последней инстанции.
export function buildPrArgs(flags, commits, branch, base = "main") {
  const args = ["pr", "create", "--base", base];
  if (!flags.title && !flags.body) {
    args.push("--fill");
    return args;
  }
  const derivedBody = commits.length ? commits.map((c) => `- ${c}`).join("\n") : "";
  const title = flags.title || commits[0] || branch; // всегда непусто
  const body = flags.body || derivedBody || flags.title || branch; // фолбэк = сам title
  args.push("--title", title, "--body", body);
  return args;
}

function pr(exec, preset, flags, { dry }) {
  const branch = assertNotMain(currentBranch(exec), preset.frame);
  const base = "main";
  // Коммиты нужны ТОЛЬКО в derive-пути (ровно один из title/body задан). Оба заданы или ни одного
  // (--fill) — чтение лишнее.
  const needDerive = (flags.title || flags.body) && !(flags.title && flags.body);
  const commits = needDerive ? branchCommits(exec, base) : [];
  const args = buildPrArgs(flags, commits, branch, base);
  mutate(exec, dry, "gh", args);
  exec.log("[git-flow] PR открыт.");
}

// frame.prRequired: приземляем ТОЛЬКО через открытый PR.
function requireOpenPr(exec) {
  const r = exec.gh(["pr", "view", "--json", "state", "-q", ".state"]);
  // stderr прокинут (#3): нет PR vs 403/auth — видно причину, не «code 1».
  if (r.code !== 0)
    throw new Error(
      `frame.prRequired: PR не проверить — gh: ${(r.err || r.out || `code ${r.code}`).trim()}`,
    );
  if (r.out.trim() !== "OPEN") throw new Error(`frame.prRequired: PR не OPEN (${r.out.trim()}).`);
}

function syncMain(exec, { dry } = {}) {
  mutate(exec, dry, "git", ["fetch", "origin", "main"]);
  mutate(exec, dry, "git", ["checkout", "main"]);
  mutate(exec, dry, "git", ["reset", "--hard", "origin/main"]);
  exec.log("[git-flow] local main = origin/main.");
}

// AUTO-MERGE (DEVOPSER-157): включает серверный auto-merge и возвращается СРАЗУ — GitHub домержит
// PR по зелёным required-checks ruleset'а и удалит ветку сам. Локально НЕ ждём checks и НЕ синкаем
// main: гейт качества — required-checks на серверной стороне (strict OFF, self-approve невозможен →
// approvals=0), а свежесть main обеспечивают start (fetch origin/main) и явный git-flow sync.
// Требует repo-настроек из пресета (allow_auto_merge + squash-only) — материализуй git-flow
// rulesets --apply, иначе `gh pr merge --auto` упадёт.
function land(exec, preset, _flags, { dry }) {
  const branch = assertNotMain(currentBranch(exec), preset.frame);
  if (preset.frame.prRequired) requireOpenPr(exec);
  mutate(exec, dry, "gh", [
    "pr",
    "merge",
    "--auto",
    mergeFlag(preset.defaults.merge),
    "--delete-branch",
  ]);
  exec.log(
    `[git-flow] ${branch}: auto-merge (${preset.defaults.merge}) включён — GitHub смержит по зелёным required-checks + удалит ветку. Синк локального main — git-flow sync после мержа.`,
  );
}

// nameWithOwner текущего репо (в переменной — литерала-плейсхолдера {owner} нет).
function repoNwo(exec) {
  return read(
    exec,
    "gh",
    ["repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"],
    "репо",
  );
}

// Каллер-джобы skeleton stack-CI — зеркалит template.json ci.jobs[*].name (+ init.mjs CI_JOB):
// go→"go", node→"node", frontend→"web", python→"python" (DEVOPSER-159). GitHub именует check-run
// reusable-caller'а "<caller-job> / <inner-job>" — ТОЛЬКО они субстантивны (сборка/тест/drift per
// stack) и годятся в required. CodeQL default-setup ("Analyze (…)"), pr-title/semantic и прочая
// инфра этого stack-префикса НЕ несут → в required не попадают by construction.
const STACK_CI_JOBS = ["go", "node", "web", "python"];
export const isStackCiCheck = (name) => STACK_CI_JOBS.some((job) => name.startsWith(`${job} / `));

// Required-контексты = РЕАЛЬНЫЕ имена stack-CI check-run'ов default-ветки (ground truth,
// DEVOPSER-117), СУЖЕННЫЕ до stack-CI (DEVOPSER-138). GitHub именует проверки reusable-caller'ов
// '<job> / <inner-job-name>', голых ключей job'а нет — берём фактические check-run'ы (не ключи
// ci.yml, самокорректируется), но оставляем лишь stack-CI (isStackCiCheck): CodeQL/pr-title/инфра
// отсеиваются, их флейк не блокирует мерж. Прогонов ещё нет → пусто (звонящий делает loud-warn).
function resolveCheckRunNames(exec, nwo) {
  const branch = read(
    exec,
    "gh",
    ["repo", "view", "--json", "defaultBranchRef", "-q", ".defaultBranchRef.name"],
    "default-ветка",
  );
  const names = read(
    exec,
    "gh",
    ["api", `repos/${nwo}/commits/${branch}/check-runs`, "-q", ".check_runs[].name"],
    "check-runs",
  );
  return [
    ...new Set(
      names
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean)
        .filter(isStackCiCheck), // сузить до stack-CI: CodeQL/pr-title/инфра — не required (DEVOPSER-138)
    ),
  ];
}

// rulesets — материализатор enforcement из git-пресета: ruleset (branch-protection) + repo-settings
// (squash-only + auto-merge, DEVOPSER-157). Дефолт = check (дрейф обоих → loud-fail); --apply
// применяет через gh api (ruleset идемпотентно PUT/POST; repo-settings PATCH). Admin-scope токен для
// apply — env-инжект (gh читает GH_TOKEN), НЕ хардкодится.
async function rulesets(exec, preset, { apply, dry }) {
  const nwo = repoNwo(exec);
  const rc = preset.defaults?.requiredChecks;
  // "from-stack" → required = РЕАЛЬНЫЕ check-run имена репо (не ключи job'ов; DEVOPSER-117).
  const checks =
    rc === "from-stack" ? resolveCheckRunNames(exec, nwo) : Array.isArray(rc) ? rc : [];
  if (rc === "from-stack" && checks.length === 0)
    exec.log(
      "[git-flow] ⚠ check-run'ов на default-ветке нет — required-checks ПУСТ. Прогони CI, затем rulesets --apply (иначе проверки не станут required).",
    );
  const desired = buildRulesetSpec(preset, checks);
  const desiredRepo = buildRepoSettings(preset); // squash-only + auto-merge (DEVOPSER-157)
  const path = `repos/${nwo}/rulesets`;
  const list = JSON.parse(read(exec, "gh", ["api", path], "rulesets") || "[]");
  const existing = list.find((r) => r.name === RULESET_NAME);
  exec.log(`[git-flow] ruleset ${RULESET_NAME}: required checks [${checks.join(", ")}].`);
  exec.log(`[git-flow] repo-settings: ${JSON.stringify(desiredRepo)}.`);

  if (apply) {
    // repo-settings ПЕРЕД ruleset: auto-merge (land --auto) требует allow_auto_merge на репо.
    if (dry) exec.log(`[dry-run] gh api repos/${nwo} --method PATCH (repo-settings)`);
    else
      mutate(
        exec,
        false,
        "gh",
        ["api", `repos/${nwo}`, "--method", "PATCH", "--input", "-"],
        JSON.stringify(desiredRepo),
      );
    exec.log("[git-flow] repo-settings применены (PATCH).");
    const method = existing ? "PUT" : "POST";
    const at = existing ? `${path}/${existing.id}` : path;
    if (dry) exec.log(`[dry-run] gh api ${at} --method ${method} (ruleset ${RULESET_NAME})`);
    else
      mutate(
        exec,
        false,
        "gh",
        ["api", at, "--method", method, "--input", "-"],
        JSON.stringify(desired),
      );
    exec.log(`[git-flow] ruleset ${RULESET_NAME} применён (${method}).`);
    return;
  }

  const currentRepo = JSON.parse(read(exec, "gh", ["api", `repos/${nwo}`], "repo") || "{}");
  const current = existing
    ? JSON.parse(read(exec, "gh", ["api", `${path}/${existing.id}`], "ruleset"))
    : null;
  const drift = [...diffRepoSettings(currentRepo, desiredRepo), ...diffRulesets(current, desired)];
  if (drift.length) {
    for (const d of drift) exec.log(`  - ${d}`);
    throw new Error(
      `ruleset/repo дрейф против git-пресета (${drift.length}) — синк: git-flow rulesets --apply`,
    );
  }
  exec.log(`[git-flow] ruleset ${RULESET_NAME} + repo-settings: совпадает с пресетом (чисто).`);
}

function parseFlags(argv) {
  const f = {};
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--title") f.title = argv[++i];
    else if (argv[i] === "--body") f.body = argv[++i];
  }
  return f;
}

// Диспатч субкоманды (экспортируется — тесты гоняют с мок-exec, без реального git).
export async function dispatch(argv, exec, preset, opts = {}) {
  const [cmd, ...rest] = argv;
  switch (cmd) {
    case "start":
      return start(exec, preset, rest[0], opts);
    case "commit":
      return commit(exec, preset, rest[0], opts);
    case "push":
      return push(exec, preset, opts);
    case "pr":
      return pr(exec, preset, parseFlags(rest), opts);
    case "land":
      return land(exec, preset, parseFlags(rest), opts);
    case "sync":
      return syncMain(exec, opts);
    case "rulesets":
      return await rulesets(exec, preset, { apply: rest.includes("--apply"), dry: opts.dry });
    default:
      throw new Error(`неизвестная команда: ${cmd} (start|commit|push|pr|land|sync|rulesets)`);
  }
}

function printHelp() {
  console.log(
    "git-flow <start|commit|push|pr|land|sync|rulesets> [args] [--dry-run]\n" +
      "  start <type>/<slug>   ветка от origin/main (по branchNaming)\n" +
      "  commit <msg>          коммит (по commitConvention)\n" +
      "  push                  push ветки в origin\n" +
      "  pr [--title --body]   открыть PR (gh)\n" +
      "  land                  включить auto-merge (по пресету, --auto) → GitHub домержит по зелёным + удалит ветку\n" +
      "  sync                  local main = origin/main\n" +
      "  rulesets [--apply]    материализовать GitHub-rulesets + repo-settings из git-пресета (дефолт: check-дрейф)",
  );
}

async function main() {
  const raw = process.argv.slice(2);
  if (!raw[0] || ["-h", "--help", "help"].includes(raw[0])) {
    printHelp();
    return;
  }
  const dry = raw.includes("--dry-run");
  const argv = raw.filter((a) => a !== "--dry-run");
  const exec = realExec();
  try {
    const preset = resolvePreset(repoRoot(exec));
    await dispatch(argv, exec, preset, { dry });
  } catch (e) {
    die(`[git-flow] ${e.message}`);
  }
}

// main только при прямом запуске (node git-flow.mjs …); при import (тесты) — не выполняется.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) main();
