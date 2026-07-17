import { describe, expect, it } from "vitest";
import { apiUrl, authHeaders } from "./api";
import type { RelationView, Rollup } from "./api";
import { relationLabel, rollupColor, rollupLabel } from "./format";

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
});
