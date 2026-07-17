import { createResource, createSignal, For, Show } from "solid-js";
import { nodeRelations } from "./api";
import type { TreeNode } from "./api";
import { relationLabel, rollupColor, rollupLabel } from "./format";

// NodeRow — рекурсивная строка дерева (read-first): раскрытие детей + бейдж роллапа +
// кросс-деп рёбра (подтягиваются по требованию при раскрытии). Без редактора — только смотрим.
export function NodeRow(props: { node: TreeNode; depth: number }) {
  // depth статичен per-инстанс (позиция в дереве не меняется) → безопасно как начальное значение.
  // eslint-disable-next-line solid/reactivity
  const [open, setOpen] = createSignal(props.depth === 0);
  const hasChildren = () => props.node.children.length > 0;

  // Связи узла тянем лениво — только когда строка раскрыта (не N запросов на старте).
  const [relations] = createResource(
    () => (open() ? props.node.key : undefined),
    (key) => nodeRelations(key),
  );

  const toggle = () => setOpen((v) => !v);

  return (
    <li class="node">
      <div class="node-row" style={{ "padding-left": `${props.depth * 1.25}rem` }}>
        <button
          class="twisty"
          classList={{ leaf: !hasChildren() }}
          onClick={toggle}
          disabled={!hasChildren()}
          aria-label={hasChildren() ? (open() ? "свернуть" : "развернуть") : "лист"}
        >
          {hasChildren() ? (open() ? "▾" : "▸") : "•"}
        </button>
        <span class="key">{props.node.key}</span>
        <span class="title">{props.node.title}</span>
        <span
          class="rollup"
          style={{ "background-color": rollupColor(props.node.rollup.category) }}
          title="свёрнутый статус/прогресс поддерева (derived)"
        >
          {rollupLabel(props.node.rollup)}
        </span>
        <For each={props.node.assignees}>{(a) => <span class="assignee">@{a}</span>}</For>
      </div>

      <Show when={open()}>
        {/* Кросс-деп рёбра узла: «зависит от WS-key (чужой)» и т.п. */}
        <Show when={(relations() ?? []).length > 0}>
          <ul class="relations" style={{ "padding-left": `${props.depth * 1.25 + 2}rem` }}>
            <For each={relations()}>
              {(rel) => (
                <li class="relation" classList={{ cross: rel.cross_workspace }}>
                  <span class="rel-kind">{rel.kind}</span>
                  <span class="rel-text">{relationLabel(rel)}</span>
                </li>
              )}
            </For>
          </ul>
        </Show>

        <Show when={hasChildren()}>
          <ul class="children">
            <For each={props.node.children}>
              {(child) => <NodeRow node={child} depth={props.depth + 1} />}
            </For>
          </ul>
        </Show>
      </Show>
    </li>
  );
}
