import { flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { h } from "vue";
import { beforeEach, expect, it, vi } from "vitest";
import { initShell, setShell, shell, SHELL_STORAGE_KEY } from "@/integrations/shell/useShell";
import AppLayout from "./AppLayout.vue";

vi.mock("./ClassicLayout.vue", () => ({
  __esModule: true,
  default: { template: '<div data-testid="classic-layout" />' },
}));
vi.mock("./MarvisLayout.vue", () => {
  throw new Error("chunk failed to load");
});

beforeEach(() => {
  localStorage.clear();
  initShell();
});

function mountLayout() {
  return mount(
    { render: () => h(NMessageProvider, null, { default: () => h(AppLayout) }) },
    { global: { config: { errorHandler: () => {} } } },
  );
}

it("falls back to the other layout when a chunk fails, without changing the saved choice", async () => {
  const wrapper = mountLayout();
  await flushPromises();
  await flushPromises();
  expect(wrapper.find('[data-testid="classic-layout"]').exists()).toBe(true);
  expect(shell.value).toBe("marvis");
  expect(localStorage.getItem(SHELL_STORAGE_KEY)).toBe("marvis");
  wrapper.unmount();
});

it("retries on an explicit switch back instead of rendering nothing", async () => {
  const wrapper = mountLayout();
  await flushPromises();
  await flushPromises();
  setShell("classic");
  await flushPromises();
  expect(wrapper.find('[data-testid="classic-layout"]').exists()).toBe(true);
  setShell("marvis");
  await flushPromises();
  await flushPromises();
  expect(wrapper.find('[data-testid="classic-layout"]').exists()).toBe(true);
  wrapper.unmount();
});
