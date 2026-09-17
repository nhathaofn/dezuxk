import * as AppBindings from "../../../wailsjs/go/main/App";

export interface GatewayStatus {
  isRunning: boolean;
  ip: string;
  port: number;
}

export async function getGatewayStatus(): Promise<GatewayStatus> {
  if (AppBindings && typeof AppBindings.GetGatewayStatus === "function") {
    try {
      const res = await AppBindings.GetGatewayStatus();
      return {
        isRunning: !!res?.isRunning,
        ip: res?.ip || "127.0.0.1",
        port: res?.port || 8080,
      };
    } catch (err) {
      console.error("Failed to get gateway status:", err);
    }
  }
  return { isRunning: false, ip: "127.0.0.1", port: 8080 };
}

export async function toggleGateway(): Promise<GatewayStatus> {
  if (AppBindings && typeof AppBindings.ToggleGateway === "function") {
    try {
      const res = await AppBindings.ToggleGateway();
      return {
        isRunning: !!res?.isRunning,
        ip: res?.ip || "127.0.0.1",
        port: res?.port || 8080,
      };
    } catch (err) {
      console.error("Failed to toggle gateway:", err);
    }
  }
  return { isRunning: false, ip: "127.0.0.1", port: 8080 };
}
