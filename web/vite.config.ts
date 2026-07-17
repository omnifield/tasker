import { defineOmnifieldVite } from "@omnifield/vite-preset";
import solid from "vite-plugin-solid";

// base ("/tasker/"), server.host, server.allowedHosts — из пресета (единый источник = omnifield.yaml).
// Продукт добавляет только свой плагин через слот + DEV-proxy (ниже). Ноль хардкода vite-специфики.
export default defineOmnifieldVite({
  plugins: [solid()],
  // DEV-ONLY: локально (без двери хаба) фронт :5173 бьёт в бэк :8030 напрямую, зеркаля дверь —
  // снимаем `/api`, нативный префикс `/tasker/`. В проде единый origin даёт дверь (:8080), proxy
  // не участвует (только dev-server). См. briefs/tasker-backlog.md (dev-заметки).
  server: {
    proxy: {
      "/api/tasker": {
        target: "http://localhost:8030",
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/api/, ""),
      },
    },
  },
});
