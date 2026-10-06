import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { beforeEach, expect, it, vi } from "vitest";
import { listAlbums, type AlbumPage, type AlbumView } from "@/api/albums";
import { useAuthStore } from "@/stores/auth";
import SidebarAlbums from "./SidebarAlbums.vue";
import { shellTestRouter, testUser } from "./testing";

vi.mock("@/api/albums", () => ({ listAlbums: vi.fn() }));

const formStub = {
  name: "AlbumFormModal",
  props: ["show", "album"],
  emits: ["update:show", "saved"],
  template: '<div v-if="show" data-testid="album-form" />',
};

function album(id: number, name: string): AlbumView {
  return { id, name, intro: "", is_public: false, cover_image_id: 0, image_count: 0 } as AlbumView;
}
function page(items: AlbumView[], total = items.length): AlbumPage {
  return { items, total, page: 1, size: 20 } as AlbumPage;
}

beforeEach(() => {
  localStorage.clear();
  vi.mocked(listAlbums).mockReset();
});

async function mountAlbums(options: { anonymous?: boolean; path?: string } = {}) {
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  if (!options.anonymous) {
    auth.token = "token";
    auth.user = testUser("user", 1);
  }
  const router = shellTestRouter();
  await router.push(options.path ?? "/upload");
  const wrapper = mount(SidebarAlbums, {
    global: { plugins: [pinia, router], stubs: { AlbumFormModal: formStub } },
  });
  await flushPromises();
  const hrefs = () =>
    wrapper.findAll('[data-testid="album-link"]').map((a) => a.attributes("href"));
  const names = () => wrapper.findAll('[data-testid="album-link"]').map((a) => a.text());
  return { wrapper, router, auth, hrefs, names };
}

it("lists albums as links", async () => {
  vi.mocked(listAlbums).mockResolvedValue(page([album(1, "旅行"), album(2, "截图")]));
  const { hrefs, names } = await mountAlbums();
  expect(listAlbums).toHaveBeenCalledWith({ page: 1, size: 20 });
  expect(hrefs()).toEqual(["/albums/1", "/albums/2"]);
  expect(names()).toEqual(["旅行", "截图"]);
});

it("always offers create and view-all entries, even with no albums", async () => {
  vi.mocked(listAlbums).mockResolvedValue(page([]));
  const { wrapper } = await mountAlbums();
  expect(wrapper.find('[data-testid="album-create"]').exists()).toBe(true);
  expect(wrapper.find('[data-testid="album-all"]').attributes("href")).toBe("/albums");
});

it("shows the total on view-all only when more albums exist than listed", async () => {
  vi.mocked(listAlbums).mockResolvedValue(page([album(1, "a"), album(2, "b")], 57));
  const many = await mountAlbums();
  expect(many.wrapper.find('[data-testid="album-all"]').text()).toContain("57");
  vi.mocked(listAlbums).mockResolvedValue(page([album(1, "a")], 1));
  const few = await mountAlbums();
  expect(few.wrapper.find('[data-testid="album-all"]').text()).not.toMatch(/\d/);
});

it("shows an inline retry on failure and recovers", async () => {
  vi.mocked(listAlbums).mockRejectedValueOnce(new Error("offline"));
  vi.mocked(listAlbums).mockResolvedValue(page([album(3, "壁纸")]));
  const { wrapper, names } = await mountAlbums();
  expect(wrapper.find('[data-testid="album-create"]').exists()).toBe(true);
  await wrapper.find('[data-testid="album-retry"]').trigger("click");
  await flushPromises();
  expect(listAlbums).toHaveBeenCalledTimes(2);
  expect(names()).toEqual(["壁纸"]);
  expect(wrapper.find('[data-testid="album-retry"]').exists()).toBe(false);
});

it("opens the existing album form and navigates to the new album after save", async () => {
  vi.mocked(listAlbums).mockResolvedValue(page([]));
  const { wrapper, router } = await mountAlbums();
  expect(wrapper.find('[data-testid="album-form"]').exists()).toBe(false);
  await wrapper.find('[data-testid="album-create"]').trigger("click");
  const form = wrapper.findComponent({ name: "AlbumFormModal" });
  expect(form.props()).toMatchObject({ show: true, album: null });
  form.vm.$emit("saved", album(9, "新相册"));
  await flushPromises();
  expect(listAlbums).toHaveBeenCalledTimes(2);
  expect(router.currentRoute.value.path).toBe("/albums/9");
});

it("drops the previous account's albums when the identity changes", async () => {
  vi.mocked(listAlbums).mockResolvedValueOnce(page([album(1, "旧账号的相册")]));
  let resolveNext!: (value: AlbumPage) => void;
  vi.mocked(listAlbums).mockReturnValueOnce(new Promise((resolve) => (resolveNext = resolve)));
  const { wrapper, auth, names } = await mountAlbums();
  expect(names()).toEqual(["旧账号的相册"]);
  auth.user = testUser("user", 2);
  await flushPromises();
  expect(wrapper.text()).not.toContain("旧账号的相册");
  resolveNext(page([album(5, "新账号的相册")]));
  await flushPromises();
  expect(names()).toEqual(["新账号的相册"]);
});

it("does not request albums for anonymous visitors", async () => {
  await mountAlbums({ anonymous: true });
  expect(listAlbums).not.toHaveBeenCalled();
});

it("ignores a stale response that resolves after a newer one", async () => {
  const resolvers: ((value: AlbumPage) => void)[] = [];
  vi.mocked(listAlbums).mockImplementation(() => new Promise((resolve) => resolvers.push(resolve)));
  const { auth, names } = await mountAlbums();
  auth.user = testUser("user", 2);
  await flushPromises();
  expect(resolvers).toHaveLength(2);
  resolvers[1]!(page([album(2, "second")]));
  await flushPromises();
  resolvers[0]!(page([album(1, "first")]));
  await flushPromises();
  expect(names()).toEqual(["second"]);
});

it("refreshes after leaving the albums page, where albums are edited and deleted", async () => {
  vi.mocked(listAlbums).mockResolvedValue(page([album(1, "a")]));
  const { router } = await mountAlbums({ path: "/albums" });
  expect(listAlbums).toHaveBeenCalledTimes(1);
  await router.push("/images");
  await flushPromises();
  expect(listAlbums).toHaveBeenCalledTimes(2);
  await router.push("/upload");
  await flushPromises();
  expect(listAlbums).toHaveBeenCalledTimes(2);
});

it("collapses and expands the list", async () => {
  vi.mocked(listAlbums).mockResolvedValue(page([album(1, "a")]));
  const { wrapper } = await mountAlbums();
  const toggle = wrapper.find('[data-testid="album-toggle"]');
  expect(toggle.attributes("aria-expanded")).toBe("true");
  await toggle.trigger("click");
  expect(toggle.attributes("aria-expanded")).toBe("false");
  expect(wrapper.find('[data-testid="album-link"]').exists()).toBe(false);
});
