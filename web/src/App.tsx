import { createEffect, createResource, createSignal, For, Show } from "solid-js";
import { listWorkspaces, workspaceTree } from "./api";
import { NodeRow } from "./TreeView";

// App — read-first обзор: список workspaces → дерево узлов выбранного (subtree-fetch) с
// роллап-бейджами и кросс-деп рёбрами. Только просмотр; создание/правка — следующая итерация.
export function App() {
  const [workspaces] = createResource(listWorkspaces);
  const [selected, setSelected] = createSignal<string | undefined>();

  // Автовыбор первого workspace, как только список приехал.
  createEffect(() => {
    const list = workspaces();
    if (list && list.length > 0 && selected() === undefined) {
      setSelected(list[0].key);
    }
  });

  const [tree] = createResource(selected, workspaceTree);

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

        <section class="tree-pane">
          <Show when={selected()} fallback={<p class="muted">выберите workspace слева</p>}>
            <h2>{selected()}</h2>
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
      </main>
    </div>
  );
}
