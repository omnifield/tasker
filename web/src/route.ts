// route.ts — hash-роутинг read-first фронта: URL = источник правды. `#/<KEY>` → узел,
// `#/<WS>` → раздел (workspace); ws выводится из ключа узла (`<WS>-<seq>`). Чистые хелперы
// (юнит-тест без DOM). Работает под base двери без её правок; чистый path — позже, когда дверь
// даст SPA-fallback (см. knowledger MECH-10, паттерн лифтнут 1:1 на общий vite+solid фронт).

// Ключ узла: <WS>-<seq> (WS — латиница с буквы). Иначе сегмент трактуем как ключ workspace.
const NODE_KEY_RE = /^[A-Za-z][A-Za-z0-9]*-\d+$/;

export interface Route {
  ws?: string; // активный workspace (из #/<WS> либо из префикса ключа узла)
  key?: string; // выбранный узел (#/<KEY>), если сегмент — ключ узла
}

// wsFromKey — префикс воркспейса из ключа узла: "TSK-34" → "TSK".
export function wsFromKey(key: string): string {
  const i = key.lastIndexOf("-");
  return i > 0 ? key.slice(0, i) : key;
}

// parseHash — разбирает location.hash в Route. Пусто/мусор → {}.
export function parseHash(hash: string): Route {
  const seg = hash.replace(/^#\/?/, "").trim();
  if (!seg) return {};
  if (NODE_KEY_RE.test(seg)) return { ws: wsFromKey(seg), key: seg };
  return { ws: seg };
}

// nodeHash/wsHash — канонический хэш узла/раздела (клик пишет его в location.hash).
export function nodeHash(key: string): string {
  return `#/${key}`;
}
export function wsHash(ws: string): string {
  return `#/${ws}`;
}
