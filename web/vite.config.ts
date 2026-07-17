import { defineOmnifieldVite } from "@omnifield/vite-preset";
import solid from "vite-plugin-solid";

// base ("/tasker/"), server.host, server.allowedHosts — из пресета (единый источник = omnifield.yaml).
// Продукт добавляет только свой плагин через слот. Ноль хардкода vite-специфики.
export default defineOmnifieldVite({
  plugins: [solid()],
});
