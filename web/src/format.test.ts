import { describe, expect, it } from "vitest";
import { apiUrl, authHeaders } from "./api";
import type { Activity, RelationView, Rollup, Status } from "./api";
import {
  activityLabel,
  activityText,
  fmtTime,
  isWaiting,
  parseSections,
  priorityLabel,
  relationLabel,
  rollupColor,
  rollupLabel,
  statusColor,
  statusName,
} from "./format";

describe("apiUrl", () => {
  it("клеит путь к door-базе с нормализацией слэша", () => {
    expect(apiUrl("/workspaces")).toBe("/api/tasker/workspaces");
    expect(apiUrl("nodes/TASK-1/relations")).toBe("/api/tasker/nodes/TASK-1/relations");
  });
});

describe("authHeaders", () => {
  it("отдаёт token-stub Bearer", () => {
    expect(authHeaders("egor")).toEqual({ Authorization: "Bearer egor" });
  });
});

describe("rollupLabel", () => {
  it("без детей — своя категория", () => {
    const r: Rollup = { has_children: false, total: 0, completed: 0, fraction: 0, category: "backlog" };
    expect(rollupLabel(r)).toBe("backlog");
  });
  it("с детьми — свёртка + прогресс", () => {
    const r: Rollup = { has_children: true, total: 4, completed: 1, fraction: 0.25, category: "in_progress" };
    expect(rollupLabel(r)).toBe("in_progress · 1/4 (25%)");
  });
  it("неизвестная категория падает на дефолт-цвет", () => {
    expect(rollupColor("zzz")).toBe(rollupColor("backlog"));
    expect(rollupColor("done")).toBe("#2ecc71");
  });
});

describe("relationLabel", () => {
  const other = { id: "x", key: "INFRA-42", title: "bridge", workspace_id: "iw", workspace_key: "INFRA" };

  it("исходящее depends_on на чужой узел помечает (чужой)", () => {
    const rel: RelationView = {
      id: "r", kind: "depends_on", direction: "outgoing", from_node: "a", to_node: "b",
      other, cross_workspace: true, created_at: "",
    };
    expect(relationLabel(rel)).toBe("зависит от INFRA-42 (чужой)");
  });

  it("входящее blocks читается как «заблокирован узлом …»", () => {
    const rel: RelationView = {
      id: "r", kind: "blocks", direction: "incoming", from_node: "a", to_node: "b",
      other, cross_workspace: true, created_at: "",
    };
    expect(relationLabel(rel)).toBe("заблокирован узлом INFRA-42 (чужой)");
  });

  it("свой workspace — без пометки", () => {
    const rel: RelationView = {
      id: "r", kind: "relates", direction: "outgoing", from_node: "a", to_node: "b",
      other: { ...other, key: "TASK-2", workspace_key: "TASK" }, cross_workspace: false, created_at: "",
    };
    expect(relationLabel(rel)).toBe("связан с TASK-2");
  });

  const mk = (kind: string, direction: "outgoing" | "incoming"): RelationView => ({
    id: "r", kind, direction, from_node: "a", to_node: "b", other, cross_workspace: false, created_at: "",
  });

  it("isWaiting: исходящее depends_on и входящее blocks — ждёт", () => {
    expect(isWaiting(mk("depends_on", "outgoing"))).toBe(true);
    expect(isWaiting(mk("blocks", "incoming"))).toBe(true);
    expect(isWaiting(mk("blocks", "outgoing"))).toBe(false);
    expect(isWaiting(mk("relates", "outgoing"))).toBe(false);
  });
});

describe("status helpers", () => {
  const st = (over: Partial<Status>): Status =>
    ({ id: "s", workspace_id: "w", category: "todo", name: "Todo", color: "", position: 0, ...over });

  it("statusColor: кастомный цвет > цвет категории > backlog", () => {
    expect(statusColor(st({ color: "#123456" }))).toBe("#123456");
    expect(statusColor(st({ color: "", category: "done" }))).toBe("#2ecc71");
    expect(statusColor(undefined)).toBe(rollupColor("backlog"));
  });

  it("statusName: имя или «—» без статуса", () => {
    expect(statusName(st({ name: "In Progress" }))).toBe("In Progress");
    expect(statusName(undefined)).toBe("—");
  });
});

describe("fmtTime", () => {
  it("RFC3339 → компактно", () => {
    expect(fmtTime("2026-07-17T19:39:58.123Z")).toBe("2026-07-17 19:39");
  });
  it("мусор — как есть", () => {
    expect(fmtTime("not-a-date")).toBe("not-a-date");
  });
});

describe("priorityLabel", () => {
  it("число → P-метка, негатив клампится в P0", () => {
    expect(priorityLabel(0)).toBe("P0");
    expect(priorityLabel(2)).toBe("P2");
    expect(priorityLabel(-1)).toBe("P0");
  });
});

describe("parseSections", () => {
  it("монолит без заголовков → один сектор heading=null", () => {
    const s = parseSections("просто текст\nвторая строка");
    expect(s).toHaveLength(1);
    expect(s[0].heading).toBeNull();
    expect(s[0].body).toBe("просто текст\nвторая строка");
  });
  it("## заголовки → секторы; текст до первого = вступление", () => {
    const s = parseSections("интро\n## Идея\nтело идеи\n## DoD\nкритерий");
    expect(s.map((x) => x.heading)).toEqual([null, "Идея", "DoD"]);
    expect(s[1].body).toBe("тело идеи");
    expect(s[2].body).toBe("критерий");
  });
  it("пустое описание → []", () => {
    expect(parseSections("")).toEqual([]);
  });
});

describe("activity helpers", () => {
  const act = (over: Partial<Activity>): Activity =>
    ({ id: "a", node_id: "n", actor: "claude", kind: "commented", created_at: "", ...over });

  it("activityLabel: известный kind → человекочитаемо, иначе сам kind", () => {
    expect(activityLabel(act({ kind: "commented" }))).toBe("комментарий");
    expect(activityLabel(act({ kind: "status_changed" }))).toBe("сменил статус");
    expect(activityLabel(act({ kind: "weird" }))).toBe("weird");
  });

  it("activityText: тянет text из data, иначе пусто", () => {
    expect(activityText(act({ data: { text: "привет" } }))).toBe("привет");
    expect(activityText(act({ data: { other: 1 } }))).toBe("");
    expect(activityText(act({ data: undefined }))).toBe("");
  });
});
