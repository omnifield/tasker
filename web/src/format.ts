// Чистые презентационные хелперы (юнит-тестируемы без DOM): бейдж роллапа + подпись кросс-деп ребра.
import type { Rollup, RelationView } from "./api";

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
