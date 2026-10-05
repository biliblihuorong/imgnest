import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it, vi } from "vitest";
import AlbumDetailView from "./AlbumDetailView.vue";
import ImageLibrary from "@/components/images/ImageLibrary.vue";
vi.mock("@/components/images/ImageLibrary.vue", () => ({
  default: {
    name: "ImageLibrary",
    props: ["lockedAlbumId", "lockedAlbumName"],
    template: "<div data-library />",
  },
}));
describe("fixed album route boundary", () => {
  it("preserves decimal route IDs beyond JS safe integers and updates same-route scope", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/albums/:id", name: "album-detail", component: AlbumDetailView },
        { path: "/albums", name: "albums", component: { template: "<div />" } },
      ],
    });
    await router.push("/albums/9007199254740993?name=Travel");
    await router.isReady();
    const wrapper = mount(AlbumDetailView, { global: { plugins: [router] } });
    expect(wrapper.findComponent(ImageLibrary).props("lockedAlbumId")).toBe("9007199254740993");
    await router.push("/albums/42?name=Work");
    await flushPromises();
    expect(wrapper.findComponent(ImageLibrary).props("lockedAlbumId")).toBe("42");
    expect(wrapper.findComponent(ImageLibrary).props("lockedAlbumName")).toBe("Work");
    wrapper.unmount();
  });
  it("does not turn malformed album IDs into a personal-library query", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/albums/:id", component: AlbumDetailView }],
    });
    await router.push("/albums/01");
    await router.isReady();
    const wrapper = mount(AlbumDetailView, { global: { plugins: [router] } });
    expect(wrapper.findComponent(ImageLibrary).exists()).toBe(false);
    wrapper.unmount();
  });
});
