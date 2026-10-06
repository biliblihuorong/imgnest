import { listAdminImages, listUsers } from "./admin";
import { listAlbums } from "./albums";
import { me } from "./auth";
import { listImages, listTrash } from "./images";

export type DashboardMetric =
  "storage" | "images" | "albums" | "trash" | "globalImages" | "accounts";
export interface DashboardIdentity {
  id: number;
  role: "admin" | "user";
}
const firstPage = { page: 1, size: 1 };

/** Every value comes from an authoritative native total, never from page length. */
export async function fetchDashboardMetric(
  metric: DashboardMetric,
  identity: DashboardIdentity,
): Promise<number> {
  if ((metric === "globalImages" || metric === "accounts") && identity.role !== "admin") {
    throw new Error("Administrative metric unavailable");
  }
  let value: unknown;
  switch (metric) {
    case "storage": {
      const account = await me();
      if (account.id !== identity.id) throw new Error("Account changed");
      value = account.used_bytes;
      break;
    }
    case "images":
      value = (await listImages(firstPage)).total;
      break;
    case "albums":
      value = (await listAlbums(firstPage)).total;
      break;
    case "trash":
      value = (await listTrash(firstPage)).total;
      break;
    case "globalImages":
      value = (await listAdminImages(firstPage)).total;
      break;
    case "accounts":
      value = (await listUsers(firstPage)).total;
      break;
  }
  if (
    typeof value !== "number" ||
    !Number.isFinite(value) ||
    value < 0 ||
    !Number.isInteger(value)
  ) {
    throw new Error("Metric unavailable");
  }
  return value;
}
