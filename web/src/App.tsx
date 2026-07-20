import { createEffect, createMemo, createResource, createSignal, For, Show } from "solid-js";
import type { NodeDTO, Status, TreeNode } from "./api";
import {
  acceptProposal,
  declineProposal,
  listInbox,
  listStatuses,
  listWorkspaces,
  workspaceTree,
} from "./api";
import { priorityColor, priorityLabel, proposalMeta, rollupColor } from "./format";
import { NodeDetail } from "./NodeDetail";
import { NodeRow } from "./TreeView";
import { TreeProvider } from "./treeCtx";

// Порядок категорий в обзорной полосе (активные ветки — слева).
const CATEGORY_ORDER = ["in_progress", "todo", "backlog", "done", "canceled"];

// App — read-first обзор роадмапа: workspaces → дерево (обзор по категориям + собственный статус +
// роллап) → панель деталей выбранной ноды. Только просмотр; правка — следующая итерация.
export function App() {
  const [workspaces] = createResource(listWorkspaces);
  const [selected, setSelected] = createSignal<string | undefined>();
  const [sel, setSel] = createSignal<TreeNode | undefined>();

  // Автовыбор первого workspace, как только список приехал.
  createEffect(() => {
    const list = workspaces();
    if (list && list.length > 0 && selected() === undefined) setSelected(list[0].key);
  });

  const [tree, { refetch: refetchTree }] = createResource(selected, workspaceTree);
  const [statuses] = createResource(selected, listStatuses);
  const [inbox, { refetch: refetchInbox }] = createResource(selected, listInbox);
  const [actionErr, setActionErr] = createSignal<string | undefined>();

  // id → Status для резолва собственного статуса узла и обзора по категориям.
  const statusMap = createMemo(() => {
    const m = new Map<string, Status>();
    for (const s of statuses() ?? []) m.set(s.id, s);
    return m;
  });
  const statusFor = (id: string | null) => (id ? statusMap().get(id) : undefined);

  // category → status_id (первый по позиции) — для дефолт-статуса при accept.
  const statusByCat = createMemo(() => {
    const m = new Map<string, string>();
    for (const s of statuses() ?? []) if (!m.has(s.category)) m.set(s.category, s.id);
    return m;
  });

  // Предложка отклонена, если её статус — категории canceled (остаётся в инбоксе как история).
  const isDeclined = (p: NodeDTO) => statusFor(p.status_id)?.category === "canceled";
  // Ожидающие решения (для счётчика) — всё, кроме отклонённых.
  const pending = createMemo(() => (inbox() ?? []).filter((p) => !isDeclined(p)));

  // accept: приземляем предложку в роадмап корнем со статусом backlog (архитектор дальше
  // приоритезирует/переносит). Обновляем инбокс + дерево (нода уходит из инбокса, входит в роадмап).
  const onAccept = async (p: NodeDTO) => {
    setActionErr(undefined);
    try {
      await acceptProposal(p.key, { status_id: statusByCat().get("backlog") ?? null });
      setSel(undefined);
      void refetchInbox();
      void refetchTree();
    } catch (e) {
      setActionErr(String(e));
    }
  };

  // decline: отклоняем с опц. комментом (canceled); остаётся в истории инбокса.
  const onDecline = async (p: NodeDTO) => {
    setActionErr(undefined);
    const comment = window.prompt(`Отклонить ${p.key}? Причина (опц.):`);
    if (comment === null) return; // отмена диалога
    try {
      await declineProposal(p.key, { comment });
      setSel(undefined);
      void refetchInbox();
    } catch (e) {
      setActionErr(String(e));
    }
  };

  // Смена workspace сбрасывает выбранную ноду (она из другого дерева) и ошибку действия.
  createEffect(() => {
    selected();
    setSel(undefined);
    setActionErr(undefined);
  });

  // Обзор: считаем узлы по категории собственного статуса (обход всего дерева).
  const overview = createMemo(() => {
    const counts: Record<string, number> = {};
    const walk = (nodes: TreeNode[]) => {
      for (const n of nodes) {
        const cat = statusFor(n.status_id)?.category ?? "backlog";
        counts[cat] = (counts[cat] ?? 0) + 1;
        walk(n.children);
      }
    };
    walk(tree() ?? []);
    return CATEGORY_ORDER.filter((c) => counts[c]).map((c) => ({ category: c, count: counts[c] }));
  });

  return (
    <div class="app">
      <header class="topbar">
        <h1>tasker</h1>
        <span class="tagline">дерево разработки — read-first</span>
      </header>

      <main class="layout">
        <nav class="sidebar">
          <h2>Workspaces</h2>
          <Show when={!workspaces.loading} fallback={<p class="muted">загрузка…</p>}>
            <Show when={!workspaces.error} fallback={<p class="error">{String(workspaces.error)}</p>}>
              <ul class="ws-list">
                <For each={workspaces()} fallback={<p class="muted">пусто — засейте workspace</p>}>
                  {(ws) => (
                    <li>
                      <button
                        class="ws-item"
                        classList={{ active: ws.key === selected() }}
                        onClick={() => setSelected(ws.key)}
                      >
                        <span class="ws-key">{ws.key}</span>
                        <span class="ws-name">{ws.name}</span>
                      </button>
                    </li>
                  )}
                </For>
              </ul>
            </Show>
          </Show>
        </nav>

        <TreeProvider value={{ statusFor, selectedKey: () => sel()?.key, select: setSel }}>
          <section class="tree-pane">
            <Show when={selected()} fallback={<p class="muted">выберите workspace слева</p>}>
              <div class="pane-head">
                <div class="view-tabs">
                  <button class="tab active">Роадмап</button>
                  <button class="tab" disabled title="скоро">Доска</button>
                  <button class="tab" disabled title="скоро">Таблица</button>
                </div>
                <div class="overview">
                  <For each={overview()}>
                    {(o) => (
                      <span class="ov-chip">
                        <span class="ov-dot" style={{ "background-color": rollupColor(o.category) }} />
                        {o.category} {o.count}
                      </span>
                    )}
                  </For>
                </div>
              </div>

              {/* Входящие — cross-product предложки (origin=proposal): вне роадмапа, с гейтом
                  accept/decline. Клик по предложке открывает её документацию (бриф) справа. */}
              <Show when={(inbox()?.length ?? 0) > 0}>
                <div class="inbox">
                  <div class="inbox-head">
                    <h3>
                      Входящие <span class="inbox-count">{pending().length}</span>
                    </h3>
                    <span class="muted">предложки других продуктов — вне роадмапа, пока не приняты</span>
                  </div>
                  <Show when={actionErr()}>
                    <p class="error">{actionErr()}</p>
                  </Show>
                  <ul class="inbox-list">
                    <For each={inbox()}>
                      {(p) => (
                        <li class="proposal" classList={{ declined: isDeclined(p) }}>
                          <button class="proposal-main" onClick={() => setSel({ ...p, children: [] })}>
                            <span
                              class="prio"
                              style={{ "border-color": priorityColor(p.priority), color: priorityColor(p.priority) }}
                            >
                              {priorityLabel(p.priority)}
                            </span>
                            <span class="key">{p.key}</span>
                            <span class="title">{p.title}</span>
                            <span class="proposal-from muted">{proposalMeta(p)}</span>
                          </button>
                          <Show
                            when={!isDeclined(p)}
                            fallback={<span class="proposal-tag">отклонено</span>}
                          >
                            <span class="proposal-actions">
                              <button class="btn accept" onClick={() => onAccept(p)}>
                                Принять
                              </button>
                              <button class="btn decline" onClick={() => onDecline(p)}>
                                Отклонить
                              </button>
                            </span>
                          </Show>
                        </li>
                      )}
                    </For>
                  </ul>
                </div>
              </Show>

              <Show when={!tree.loading} fallback={<p class="muted">загрузка дерева…</p>}>
                <Show when={!tree.error} fallback={<p class="error">{String(tree.error)}</p>}>
                  <ul class="tree">
                    <For each={tree()} fallback={<p class="muted">нет корневых узлов</p>}>
                      {(root) => <NodeRow node={root} depth={0} />}
                    </For>
                  </ul>
                </Show>
              </Show>
            </Show>
          </section>

          <Show when={sel()} fallback={<aside class="detail empty"><p class="muted">выберите ноду — покажу документацию, статус, зависимости и историю</p></aside>}>
            {(node) => <NodeDetail node={node()} />}
          </Show>
        </TreeProvider>
      </main>
    </div>
  );
}
