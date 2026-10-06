import { flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { h } from "vue";
import { beforeEach, expect, it, vi } from "vitest";
import { initShell, setShell } from "@/integrations/shell/useShell";
import AppLayout from "./AppLayout.vue";

vi.mock("./ClassicLayout.vue", () => ({
  __esModule: true,
  default: { template: '<div data-testid="classic-layout" />' },
}));
vi.mock("./MarvisLayout.vue", () => ({
  __esModule: true,
  default: { template: '<div data-testid="marvis-layout" />' },
}));

beforeEach(() => {
  localStorage.clear();
  initShell();
});

function mountLayout() {
  return mount({ render: () => h(NMessageProvider, null, { default: () => h(AppLayout) }) });
}

it("renders the marvis layout by default", async () => {
  const wrapper = mountLayout();
  await flushPromises();
  expect(wrapper.find('[data-testid="marvis-layout"]').exists()).toBe(true);
  expect(wrapper.find('[data-testid="classic-layout"]').exists()).toBe(false);
  wrapper.unmount();
});

it("swaps layouts without remounting the app when the shell changes", async () => {
  const wrapper = mountLayout();
  await flushPromises();
  setShell("classic");
  await flushPromises();
  expect(wrapper.find('[data-testid="classic-layout"]').exists()).toBe(true);
  expect(wrapper.find('[data-testid="marvis-layout"]').exists()).toBe(false);
  wrapper.unmount();
});
