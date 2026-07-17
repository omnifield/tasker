import { createContext, useContext } from "solid-js";
import type { Status, TreeNode } from "./api";

// TreeCtx — общий контекст дерева: резолв статуса по id (для собственного бейджа узла) и
// выбор ноды (клик по строке → панель деталей). Избегаем прокидывания пропсов через рекурсию.
export interface TreeCtx {
  statusFor: (id: string | null) => Status | undefined;
  selectedKey: () => string | undefined;
  select: (node: TreeNode) => void;
}

const Ctx = createContext<TreeCtx>();

export const TreeProvider = Ctx.Provider;

export function useTree(): TreeCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error("useTree вне TreeProvider");
  return v;
}
