import { createSignal, For, Show } from "solid-js";
import type { TreeNode } from "./api";
import { priorityColor, priorityLabel, rollupColor, rollupLabel, statusColor, statusName } from "./format";
import { useTree } from "./treeCtx";

// NodeRow — рекурсивная строка роадмап-дерева (read-first): раскрытие детей, собственный статус,
// свёрнутый роллап, метки, исполнители. Клик по строке → выбор ноды (панель деталей справа).
export function NodeRow(props: { node: TreeNode; depth: number }) {
  const tree = useTree();
  // depth статичен per-инстанс (позиция в дереве не меняется) → безопасно как начальное значение.
  // eslint-disable-next-line solid/reactivity
  const [open, setOpen] = createSignal(props.depth < 2);
  const hasChildren = () => props.node.children.length > 0;
  const status = () => tree.statusFor(props.node.status_id);
  const selected = () => tree.selectedKey() === props.node.key;

  return (
    <li class="node">
      <div class="node-row" classList={{ selected: selected() }} style={{ "padding-left": `${props.depth * 1.1}rem` }}>
        <button
          class="twisty"
          classList={{ leaf: !hasChildren() }}
          onClick={() => setOpen((v) => !v)}
          disabled={!hasChildren()}
          aria-label={hasChildren() ? (open() ? "свернуть" : "развернуть") : "лист"}
        >
          {hasChildren() ? (open() ? "▾" : "▸") : "•"}
        </button>

        <button class="node-main" onClick={() => tree.select(props.node.key)}>
          <span class="prio" style={{ "border-color": priorityColor(props.node.priority), color: priorityColor(props.node.priority) }}>
            {priorityLabel(props.node.priority)}
          </span>
          <span class="key">{props.node.key}</span>
          <span class="title">{props.node.title}</span>

          <span class="status" style={{ "background-color": statusColor(status()) }}>
            {statusName(status())}
          </span>

          <Show when={props.node.rollup.has_children}>
            <span
              class="rollup"
              style={{ "background-color": rollupColor(props.node.rollup.category) }}
              title="свёрнутый прогресс поддерева (derived)"
            >
              {rollupLabel(props.node.rollup)}
            </span>
          </Show>

          <For each={props.node.labels}>
            {(l) => <span class="label-dot" style={{ "background-color": l.color || "#666" }} title={l.name} />}
          </For>
          <For each={props.node.assignees}>{(a) => <span class="assignee">@{a}</span>}</For>
        </button>
      </div>

      <Show when={open() && hasChildren()}>
        <ul class="children">
          <For each={props.node.children}>{(child) => <NodeRow node={child} depth={props.depth + 1} />}</For>
        </ul>
      </Show>
    </li>
  );
}
