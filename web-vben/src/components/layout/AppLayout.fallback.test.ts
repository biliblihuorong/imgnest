import { flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { h } from "vue";
import { expect, it, vi } from "vitest";
import { initShell, shell } from "@/integrations/shell/useShell";
import AppLayout from "./AppLayout.vue";

vi.mock("./ClassicLayout.vue", () => ({
  __esModule: true,
  default: { template: '<div data-testid="classic-layout" />' },
}));
vi.mock("./MarvisLayout.vue", () => {
  throw new Error("chunk failed to load");
});

it("falls back to the other layout when a chunk fails to load", async () => {
  localStorage.clear();
  initShell();
  const wrapper = mount(
    { render: () => h(NMessageProvider, null, { default: () => h(AppLayout) }) },
    { global: { config: { errorHandler: () => {} } } },
  );
  await flushPromises();
  await flushPromises();
  expect(shell.value).toBe("classic");
  expect(wrapper.find('[data-testid="classic-layout"]').exists()).toBe(true);
  wrapper.unmount();
});
