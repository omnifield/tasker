// Чистые презентационные хелперы (юнит-тестируемы без DOM): бейджи статуса/роллапа, подписи
// связей и активности, форматирование времени.
import type { Rollup, RelationView, Status, Activity } from "./api";

// Цвета фикс-категорий (зеркало сид-статусов бэка).
const CATEGORY_COLORS: Record<string, string> = {
  backlog: "#95a5a6",
  todo: "#3498db",
  in_progress: "#f1c40f",
  done: "#2ecc71",
  canceled: "#e74c3c",
};

export function rollupColor(category: string): string {
  return CATEGORY_COLORS[category] ?? CATEGORY_COLORS.backlog;
}

// rollupLabel: без детей — просто своя категория; с детьми — свёртка + прогресс "done/total (NN%)".
export function rollupLabel(r: Rollup): string {
  if (!r.has_children) return r.category;
  const pct = Math.round(r.fraction * 100);
  return `${r.category} · ${r.completed}/${r.total} (${pct}%)`;
}

// relationLabel — человекочитаемое ребро ОТНОСИТЕЛЬНО узла (направленность + «(чужой)» для cross-ws).
// depends_on/blocks направлены; relates/duplicate — симметричны. Пример: «заблокирован узлом INFRA-42 (чужой)».
export function relationLabel(rel: RelationView): string {
  const other = rel.cross_workspace ? `${rel.other.key} (чужой)` : rel.other.key;
  const phrasing: Record<string, [string, string]> = {
    // kind → [исходящее (узел=from), входящее (узел=to)]
    depends_on: [`зависит от ${other}`, `нужен для ${other}`],
    blocks: [`блокирует ${other}`, `заблокирован узлом ${other}`],
    relates: [`связан с ${other}`, `связан с ${other}`],
    duplicate: [`дублирует ${other}`, `дубль ${other}`],
  };
  const pair = phrasing[rel.kind] ?? [`${rel.kind} → ${other}`, `${rel.kind} ← ${other}`];
  return rel.direction === "outgoing" ? pair[0] : pair[1];
}

// isWaiting — ребро означает, что ЭТОТ узел ждёт другой (нельзя начинать): исходящее depends_on
// (узел зависит от другого) или входящее blocks (другой блокирует узел). Derived-«blocked» на
// бэке пока нет (см. бэклог) — это визуальный признак зависимости, без проверки статуса блокера.
export function isWaiting(rel: RelationView): boolean {
  return (
    (rel.kind === "depends_on" && rel.direction === "outgoing") ||
    (rel.kind === "blocks" && rel.direction === "incoming")
  );
}

// statusColor — цвет собственного статуса узла: кастомный color статуса, иначе цвет категории,
// иначе дефолт. undefined-статус (узел без статуса) -> цвет backlog.
export function statusColor(status?: Status): string {
  if (status?.color) return status.color;
  return rollupColor(status?.category ?? "backlog");
}

// statusName — подпись статуса; для узла без статуса — «—».
export function statusName(status?: Status): string {
  return status?.name ?? "—";
}

// fmtTime — компактное время из RFC3339 ("2026-07-17T19:39:58Z" -> "2026-07-17 19:39").
// Парсим строку, не Date (стабильно/локаль-независимо для юнит-тестов).
export function fmtTime(iso: string): string {
  const m = iso.match(/^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})/);
  return m ? `${m[1]} ${m[2]}` : iso;
}

// activityLabel — человекочитаемый вид kind записи timeline.
export function activityLabel(a: Activity): string {
  const map: Record<string, string> = {
    created: "создан",
    status_changed: "сменил статус",
    assigned: "назначен",
    relation_added: "добавил связь",
    commented: "комментарий",
  };
  return map[a.kind] ?? a.kind;
}

// activityText — текст комментария из data (kind=commented кладёт {text}); иначе пусто.
export function activityText(a: Activity): string {
  if (a.data && typeof a.data === "object" && "text" in a.data) {
    const t = (a.data as { text?: unknown }).text;
    if (typeof t === "string") return t;
  }
  return "";
}
