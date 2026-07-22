import { createMemo, createResource, For, Show } from "solid-js";
import type { NodeDTO } from "./api";
import { nodeActivity, nodeRelations } from "./api";
import {
  activityLabel,
  activityText,
  fmtTime,
  isWaiting,
  parseSections,
  priorityColor,
  priorityLabel,
  relationLabel,
  rollupColor,
  rollupLabel,
  statusColor,
  statusName,
} from "./format";
import { useTree } from "./treeCtx";

// NodeDetail — правая панель выбранной ноды: вся документация и связи, что отдаёт бэк. Узел тянется
// по key (может быть вне дерева — кросс-ws реф/deep-link); связи и историю тоже по key.
export function NodeDetail(props: { node: NodeDTO }) {
  const tree = useTree();
  const status = () => tree.statusFor(props.node.status_id);
  const [relations] = createResource(() => props.node.key, nodeRelations);
  const [activity] = createResource(() => props.node.key, nodeActivity);
  const sections = createMemo(() => parseSections(props.node.description));

  return (
    <aside class="detail">
      <div class="detail-head">
        <span class="key">{props.node.key}</span>
        <span class="status" style={{ "background-color": statusColor(status()) }}>
          {statusName(status())}
        </span>
        <Show when={props.node.rollup.has_children}>
          <span class="rollup" style={{ "background-color": rollupColor(props.node.rollup.category) }}>
            {rollupLabel(props.node.rollup)}
          </span>
        </Show>
      </div>
      <h2 class="detail-title">{props.node.title}</h2>

      <div class="meta">
        <span class="prio" style={{ "border-color": priorityColor(props.node.priority), color: priorityColor(props.node.priority) }}>
          {priorityLabel(props.node.priority)}
        </span>
        <For each={props.node.labels}>
          {(l) => (
            <span class="label-chip" style={{ "border-color": l.color || "#666" }}>
              {l.name}
            </span>
          )}
        </For>
        <For each={props.node.assignees}>{(a) => <span class="assignee">@{a}</span>}</For>
      </div>

      {/* Документация ноды — секторы по Markdown-заголовкам (## …). Типизированные блоки-сущности
          придут с канон-пресетом; пока лёгкий рендер: монолит без заголовков = один сектор. */}
      <section class="detail-section">
        <h3>Документация</h3>
        <Show when={props.node.description} fallback={<p class="muted">нет описания</p>}>
          <For each={sections()}>
            {(sec) => (
              <Show when={sec.heading} fallback={<p class="doc">{sec.body}</p>}>
                <details class="sector" open>
                  <summary>{sec.heading}</summary>
                  <p class="doc">{sec.body}</p>
                </details>
              </Show>
            )}
          </For>
        </Show>
      </section>

      {/* Зависимости — связи с другими нодами (в т.ч. чужой воркспейс, направленность). */}
      <section class="detail-section">
        <h3>Зависимости</h3>
        <Show when={!relations.loading} fallback={<p class="muted">загрузка…</p>}>
          <Show when={(relations() ?? []).length > 0} fallback={<p class="muted">нет связей</p>}>
            <ul class="dep-list">
              <For each={relations()}>
                {(rel) => (
                  <li class="dep" classList={{ cross: rel.cross_workspace, waiting: isWaiting(rel) }}>
                    <span class="rel-kind">{rel.kind}</span>
                    {/* Клик по связи ведёт на её узел (по человеческому key, в т.ч. чужой ws). */}
                    <button
                      class="dep-link"
                      onClick={() => tree.select(rel.other.key)}
                      title={`перейти к ${rel.other.key}`}
                    >
                      <span class="dep-text">{relationLabel(rel)}</span>
                      <span class="dep-title muted">{rel.other.title}</span>
                    </button>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </Show>
      </section>

      {/* История ноды (typed timeline). */}
      <section class="detail-section">
        <h3>История</h3>
        <Show when={!activity.loading} fallback={<p class="muted">загрузка…</p>}>
          <Show when={(activity() ?? []).length > 0} fallback={<p class="muted">пусто</p>}>
            <ul class="timeline">
              <For each={activity()}>
                {(a) => (
                  <li class="event">
                    <span class="event-time muted">{fmtTime(a.created_at)}</span>
                    <span class="event-actor">@{a.actor}</span>
                    <span class="event-kind">{activityLabel(a)}</span>
                    <Show when={activityText(a)}>
                      <span class="event-text">{activityText(a)}</span>
                    </Show>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </Show>
      </section>
    </aside>
  );
}
