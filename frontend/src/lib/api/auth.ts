import * as AppBindings from "../../../wailsjs/go/main/App";
import type { User } from "@/types";

export async function loginUser(username: string, password: string): Promise<User> {
  if (AppBindings && typeof AppBindings.Login === "function") {
    const res = await AppBindings.Login(username, password);
    return {
      id: res.id,
      username: res.username,
      role: res.role,
      created_at: res.created_at ? String(res.created_at) : undefined,
    };
  }
  throw new Error("Wails desktop runtime is not connected.");
}

export async function registerUser(username: string, password: string): Promise<User> {
  if (AppBindings && typeof AppBindings.Register === "function") {
    const res = await AppBindings.Register(username, password);
    return {
      id: res.id,
      username: res.username,
      role: res.role,
      created_at: res.created_at ? String(res.created_at) : undefined,
    };
  }
  throw new Error("Wails desktop runtime is not connected.");
}

export async function getCurrentUser(): Promise<User | null> {
  if (AppBindings && typeof AppBindings.GetCurrentUser === "function") {
    const res = await AppBindings.GetCurrentUser();
    if (!res || !res.username) return null;
    return {
      id: res.id,
      username: res.username,
      role: res.role,
      created_at: res.created_at ? String(res.created_at) : undefined,
    };
  }
  return null;
}

export async function isAuthSetupRequired(): Promise<boolean> {
  if (AppBindings && typeof AppBindings.IsAuthSetupRequired === "function") {
    return await AppBindings.IsAuthSetupRequired();
  }
  return false;
}

export async function logoutUser(): Promise<boolean> {
  if (AppBindings && typeof AppBindings.Logout === "function") {
    return await AppBindings.Logout();
  }
  return true;
}
