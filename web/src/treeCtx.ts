import { createContext, useContext } from "solid-js";
import type { Status } from "./api";

// TreeCtx — общий контекст дерева: резолв статуса по id (для собственного бейджа узла) и
// навигация на ноду по key (клик по строке/рефу → URL-хэш → панель деталей). Избегаем
// прокидывания пропсов через рекурсию. select принимает key (не узел): цель рефа может быть
// из чужого воркспейса и вне загруженного дерева — деталь тянется по key.
export interface TreeCtx {
  statusFor: (id: string | null) => Status | undefined;
  selectedKey: () => string | undefined;
  select: (key: string) => void;
}

const Ctx = createContext<TreeCtx>();

export const TreeProvider = Ctx.Provider;

export function useTree(): TreeCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error("useTree вне TreeProvider");
  return v;
}
