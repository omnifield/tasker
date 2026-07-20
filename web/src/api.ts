// API-клиент tasker-фронта. Single-origin: зовём backend через ДВЕРЬ-контракт `/api/tasker/…`
// (дверь снимает `/api` → нативный `tasker:8030/tasker/…`), не через vite-proxy. Auth —
// token-stub `Bearer <handle>` (как chater/бэк v0); реальная identity — позже (мост).

export const API_BASE = "/api/tasker";

// token-stub: любой непустой handle проходит auth-middleware бэка. Позже — токен моста.
export const TOKEN = "web";

// --- канон-DTO (зеркало service-слоя бэка) ---------------------------------

export interface Workspace {
  id: string;
  key: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
}

export interface Rollup {
  has_children: boolean;
  total: number;
  completed: number;
  fraction: number;
  category: string;
}

export interface Label {
  id: string;
  name: string;
  color: string;
}

export interface NodeDTO {
  id: string;
  workspace_id: string;
  key: string;
  seq: number;
  parent_id: string | null;
  title: string;
  description: string;
  status_id: string | null;
  priority: number;
  // origin: "native" (роадмап-узел) или "proposal" (входящее cross-product предложение,
  // не в роадмапе, пока не принято). proposed_by/source_ws — провенанс предложки.
  origin: string;
  proposed_by?: string;
  source_ws?: string;
  rollup: Rollup;
  labels: Label[];
  assignees: string[];
}

// TreeNode — узел + рекурсивно поддерево (subtree-fetch отдаёт всё дерево одним запросом).
export interface TreeNode extends NodeDTO {
  children: TreeNode[];
}

export interface RelatedNode {
  id: string;
  key: string;
  title: string;
  workspace_id: string;
  workspace_key: string;
}

// RelationView — обогащённая связь относительно узла: направление + чужой конец + cross-ws.
export interface RelationView {
  id: string;
  kind: string;
  direction: "outgoing" | "incoming";
  from_node: string;
  to_node: string;
  other: RelatedNode;
  cross_workspace: boolean;
  created_at: string;
}

// Status — per-workspace статус: фикс-категория + кастомные name/color.
export interface Status {
  id: string;
  workspace_id: string;
  category: string;
  name: string;
  color: string;
  position: number;
}

// Activity — запись timeline (created/status_changed/assigned/relation_added/commented/…).
export interface Activity {
  id: string;
  node_id: string;
  actor: string;
  kind: string;
  data?: unknown;
  created_at: string;
}

// --- чистые хелперы (юнит-тестируемы без сети) -----------------------------

// apiUrl клеит путь к door-базе (нормализует ведущий слэш).
export function apiUrl(path: string): string {
  return `${API_BASE}${path.startsWith("/") ? path : `/${path}`}`;
}

// authHeaders — token-stub заголовок для всех запросов.
export function authHeaders(token: string = TOKEN): Record<string, string> {
  return { Authorization: `Bearer ${token}` };
}

// --- транспорт -------------------------------------------------------------

async function get<T>(path: string): Promise<T> {
  const res = await fetch(apiUrl(path), { headers: authHeaders() });
  if (!res.ok) {
    throw new Error(`GET ${path} → ${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(apiUrl(path), {
    method: "POST",
    headers: { ...authHeaders(), "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  });
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const j = (await res.json()) as { error?: string };
      if (j.error) detail = j.error;
    } catch {
      /* тело не JSON — оставляем statusText */
    }
    throw new Error(`POST ${path} → ${res.status} ${detail}`);
  }
  return (await res.json()) as T;
}

export const listWorkspaces = () => get<Workspace[]>("/workspaces");
export const workspaceTree = (ws: string) => get<TreeNode[]>(`/workspaces/${ws}/tree`);
export const listStatuses = (ws: string) => get<Status[]>(`/workspaces/${ws}/statuses`);
export const nodeRelations = (key: string) => get<RelationView[]>(`/nodes/${key}/relations`);
export const nodeActivity = (key: string) => get<Activity[]>(`/nodes/${key}/activity`);

// --- cross-product предложки (инбокс + гейт) -------------------------------

export const listInbox = (ws: string) => get<NodeDTO[]>(`/workspaces/${ws}/inbox`);

// acceptProposal приземляет предложку в роадмап (origin→native): parent/status опц.
export const acceptProposal = (key: string, body: { parent_id?: string | null; status_id?: string | null }) =>
  post<NodeDTO>(`/nodes/${key}/accept`, body);

// declineProposal отклоняет предложку (canceled + коммент); остаётся в истории инбокса.
export const declineProposal = (key: string, body: { comment?: string }) =>
  post<NodeDTO>(`/nodes/${key}/decline`, body);
