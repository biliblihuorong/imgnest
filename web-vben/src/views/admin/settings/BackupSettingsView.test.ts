import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider, type MessageProviderInst } from "naive-ui";
import { defineComponent, h, onBeforeUnmount, ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@vben/locales";
import * as backupsApi from "@/api/backups";
import type { BackupOverview } from "@/api/backups";
import { ApiError } from "@/api/client";
import BackupSettingsView from "./BackupSettingsView.vue";

vi.mock("@/api/backups", () => ({
  getBackups: vi.fn(),
  createBackup: vi.fn(),
  deleteBackup: vi.fn(),
  downloadBackup: vi.fn(),
  saveBackupSchedule: vi.fn(),
}));

const api = vi.mocked(backupsApi);
enableAutoUnmount(afterEach);

/** Naive UI 2.45.3 does not cancel message timers on provider unmount. */
const Host = defineComponent({
  setup() {
    const messages = ref<MessageProviderInst | null>(null);
    onBeforeUnmount(() => messages.value?.destroyAll());
    return () => h(NMessageProvider, { ref: messages }, () => h(BackupSettingsView));
  },
});

function overview(partial: Partial<BackupOverview> = {}): BackupOverview {
  return {
    items: [
      {
        name: "imgnest-20261016-080000-auto.zip",
        size: 2048,
        created_at: "2026-10-16T08:00:00Z",
        auto: true,
      },
      {
        name: "imgnest-20261015-080000.zip",
        size: 1024,
        created_at: "2026-10-15T08:00:00Z",
        auto: false,
      },
    ],
    schedule: { enabled: false, interval_hours: 24, keep: 7 },
    running: false,
    location: "data/backup",
    ...partial,
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  i18n.global.locale.value = "zh-CN";
  api.getBackups.mockResolvedValue(overview());
});

describe("BackupSettingsView", () => {
  it("加载失败时仍明确提示归档不含图片文件", async () => {
    api.getBackups.mockRejectedValueOnce(new Error("offline"));
    const wrapper = mount(Host);
    await flushPromises();
    expect(wrapper.get("[data-backup-exclusions]").text()).toContain("不包含图片文件");
    expect(wrapper.get("[data-backup-exclusions]").text()).toContain("本机存储目录或对象存储桶");
    expect(wrapper.find("[data-backups-error]").exists()).toBe(true);
  });

  it("lists archives with their type and the backup directory", async () => {
    const wrapper = mount(Host);
    await flushPromises();
    const rows = wrapper.findAll("[data-backup]");
    expect(rows.map((row) => row.attributes("data-backup"))).toEqual([
      "imgnest-20261016-080000-auto.zip",
      "imgnest-20261015-080000.zip",
    ]);
    expect(rows[0]!.text()).toContain("定时");
    expect(rows[1]!.text()).toContain("手动");
    expect(wrapper.get("[data-backup-location]").text()).toContain("data/backup");
    expect(wrapper.text()).toContain("主密钥");
  });

  it("creates a backup and reloads the list", async () => {
    api.createBackup.mockResolvedValue({
      name: "imgnest-20261016-090000.zip",
      size: 10,
      created_at: "2026-10-16T09:00:00Z",
      auto: false,
    });
    const wrapper = mount(Host);
    await flushPromises();
    await wrapper.get('[data-action="create-backup"]').trigger("click");
    await flushPromises();
    expect(api.createBackup).toHaveBeenCalledTimes(1);
    expect(api.getBackups).toHaveBeenCalledTimes(2);
  });

  it("explains a busy backup instead of a bare status code", async () => {
    api.createBackup.mockRejectedValue(new ApiError(30020, "busy", 409));
    const wrapper = mount(Host);
    await flushPromises();
    await wrapper.get('[data-action="create-backup"]').trigger("click");
    await flushPromises();
    expect(document.body.textContent).toContain("已有一个备份正在进行");
  });

  it("downloads through an object URL so the token stays out of the link", async () => {
    api.downloadBackup.mockResolvedValue(new Blob(["zip"]));
    const createObjectURL = vi.fn(() => "blob:backup");
    URL.createObjectURL = createObjectURL;
    URL.revokeObjectURL = vi.fn();
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    const wrapper = mount(Host);
    await flushPromises();
    await wrapper.get('[data-action="download-backup"]').trigger("click");
    await flushPromises();
    expect(api.downloadBackup).toHaveBeenCalledWith("imgnest-20261016-080000-auto.zip");
    expect(click).toHaveBeenCalledTimes(1);
    expect((click.mock.contexts[0] as HTMLAnchorElement).download).toBe(
      "imgnest-20261016-080000-auto.zip",
    );
    click.mockRestore();
  });

  it("saves the schedule", async () => {
    api.saveBackupSchedule.mockImplementation(async (schedule) => schedule);
    const wrapper = mount(Host);
    await flushPromises();
    await wrapper.get("[data-backup-schedule-enabled]").trigger("click");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.saveBackupSchedule).toHaveBeenCalledWith({
      enabled: true,
      interval_hours: 24,
      keep: 7,
    });
  });

  it("shows an empty state", async () => {
    api.getBackups.mockResolvedValue(overview({ items: [] }));
    const wrapper = mount(Host);
    await flushPromises();
    expect(wrapper.find("[data-backups-empty]").exists()).toBe(true);
  });
});
