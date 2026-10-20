// Serve the production SPA without loading or rewriting the development config.
import { fileURLToPath } from "node:url";
import { preview } from "../node_modules/vite/dist/node/index.js";

const server = await preview({
  root: fileURLToPath(new URL("../", import.meta.url)),
  configFile: false,
  preview: { host: "127.0.0.1", port: 4173, strictPort: true },
});
server.printUrls();
