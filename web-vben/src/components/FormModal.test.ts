import { mount } from "@vue/test-utils";
import { afterEach, expect, it } from "vitest";
import { nextTick } from "vue";
import FormModal from "./FormModal.vue";

afterEach(() => {
  document.body.innerHTML = "";
});

function mountModal(props: Record<string, unknown> = {}) {
  return mount(FormModal, {
    attachTo: document.body,
    props: { show: true, title: "编辑", ...props },
    slots: {
      default: '<form class="form-grid"><div data-testid="field">a</div></form>',
      footer: '<button data-testid="footer-btn">保存</button>',
    },
  });
}

const card = () => document.querySelector<HTMLElement>(".form-modal");

it("renders a centered card with the column count and width", async () => {
  mountModal({ cols: 3, width: 720 });
  await nextTick();
  const element = card();
  expect(element).not.toBeNull();
  expect(element!.style.getPropertyValue("--form-cols")).toBe("3");
  expect(element!.style.width).toContain("720px");
  expect(document.querySelector('[data-testid="field"]')).not.toBeNull();
  expect(document.querySelector('[data-testid="footer-btn"]')).not.toBeNull();
});

it("closes from the close button when idle", async () => {
  const wrapper = mountModal();
  await nextTick();
  document.querySelector<HTMLElement>(".n-base-close")!.click();
  expect(wrapper.emitted("update:show")?.at(-1)).toEqual([false]);
});

it("cannot be closed while busy", async () => {
  const wrapper = mountModal({ busy: true });
  await nextTick();
  expect(document.querySelector(".n-base-close")).toBeNull();
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  await nextTick();
  expect(wrapper.emitted("update:show")).toBeUndefined();
});
