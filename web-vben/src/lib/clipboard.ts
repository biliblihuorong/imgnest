/** 写入剪贴板的统一入口；成功与否由调用方决定提示文案。 */
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
