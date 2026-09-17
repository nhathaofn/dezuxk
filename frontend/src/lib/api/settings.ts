import * as AppBindings from "../../../wailsjs/go/main/App";

export interface AppSettings {
  gatewayPort: number;
  gatewayIP: string;
  isRunning: boolean;
}

export async function getSettings(): Promise<AppSettings> {
  if (AppBindings && typeof AppBindings.GetSettings === "function") {
    try {
      const res = await AppBindings.GetSettings();
      return {
        gatewayPort: res?.gatewayPort || 8080,
        gatewayIP: res?.gatewayIP || "127.0.0.1",
        isRunning: !!res?.isRunning,
      };
    } catch (err) {
      console.error("Failed to get settings:", err);
    }
  }
  return { gatewayPort: 8080, gatewayIP: "127.0.0.1", isRunning: false };
}

export async function updateGatewayPort(port: number): Promise<void> {
  if (AppBindings && typeof AppBindings.UpdateGatewayPort === "function") {
    await AppBindings.UpdateGatewayPort(port);
    return;
  }
  throw new Error("Wails runtime not connected");
}

export async function changePassword(currentPass: string, newPass: string): Promise<void> {
  if (AppBindings && typeof AppBindings.ChangePassword === "function") {
    await AppBindings.ChangePassword(currentPass, newPass);
    return;
  }
  throw new Error("Wails runtime not connected");
}
