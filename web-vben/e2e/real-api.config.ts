import { readFileSync } from "node:fs";
import { defineConfig } from "vitest/config";

const fixturePath = process.env.IMGNEST_REAL_API_FIXTURE;
if (!fixturePath) throw new Error("Run e2e/run_real_api.py to provision an isolated server");
const fixture = JSON.parse(readFileSync(fixturePath, "utf8")) as { origin: string };

// Deliberately separate from the mocked component/unit suite and browser E2E.
export default defineConfig({
  resolve: { alias: { "@": new URL("../src", import.meta.url).pathname } },
  test: {
    environment: "jsdom",
    environmentOptions: { jsdom: { url: fixture.origin } },
    include: ["e2e/real-api.test.ts"],
    setupFiles: ["e2e/real-api.setup.ts"],
    fileParallelism: false,
    bail: 1,
    hookTimeout: 60000,
    testTimeout: 30000,
  },
});
