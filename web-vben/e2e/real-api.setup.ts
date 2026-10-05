import { readFileSync, writeFileSync } from "node:fs";
import { afterAll } from "vitest";

export interface RealApiFixture {
  origin: string;
  nonce: string;
  adminEmail: string;
  adminPassword: string;
  userEmail: string;
  userPassword: string;
  nextPassword: string;
  images: string[];
  secretsPath: string;
  transportPath: string;
}

const fixturePath = process.env.IMGNEST_REAL_API_FIXTURE;
if (!fixturePath) throw new Error("Run e2e/run_real_api.py to provision an isolated server");
export const fixture = JSON.parse(readFileSync(fixturePath, "utf8")) as RealApiFixture;
const origin = new URL(fixture.origin);
if (origin.protocol !== "http:" || origin.hostname !== "127.0.0.1" || !origin.port) {
  throw new Error("The real API runner accepts only its ephemeral loopback server");
}

if (window.location.origin !== origin.origin)
  throw new Error("jsdom origin differs from Go server");

// Node fetch cannot resolve the browser's relative URLs. This adapter ONLY
// resolves those URLs and forwards to the original native fetch over real TCP.
// It never mocks a response, route, service, status, header, body, or delay.
// Uploads use jsdom's real XMLHttpRequest network implementation, unmodified.
const nativeFetch = globalThis.fetch;
const transport: { method: string; path: string; status: number }[] = [];
globalThis.fetch = async (input, init) => {
  const url = new URL(input instanceof Request ? input.url : String(input), origin);
  if (url.origin !== origin.origin) throw new Error("External requests are not allowed");
  const response = await nativeFetch(input instanceof Request ? input : url, init);
  transport.push({ method: init?.method ?? "GET", path: url.pathname, status: response.status });
  return response;
};

export const sensitiveValues = [fixture.adminPassword, fixture.userPassword, fixture.nextPassword];

afterAll(() => {
  // Only the Python supervisor reads this temporary file; it is never published.
  writeFileSync(fixture.secretsPath, JSON.stringify(sensitiveValues), { mode: 0o600 });
  writeFileSync(fixture.transportPath, JSON.stringify(transport));
  globalThis.fetch = nativeFetch;
});
