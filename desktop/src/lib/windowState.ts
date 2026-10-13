import { getCurrentWindow } from "@tauri-apps/api/window";

export async function windowShown(): Promise<boolean> {
  try {
    const w = getCurrentWindow();
    const [visible, minimized] = await Promise.all([w.isVisible(), w.isMinimized()]);
    return visible !== false && minimized !== true;
  } catch {
    return true;
  }
}
