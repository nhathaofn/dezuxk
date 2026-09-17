export type ToastType = "success" | "error" | "warning" | "info";

export interface ToastData {
  id: string;
  type: ToastType;
  message: string;
  description?: string;
  duration?: number; // Thời gian hiển thị mặc định 2000ms (2s)
}

type ToastListener = (toasts: ToastData[]) => void;

class ToastManager {
  private toasts: ToastData[] = [];
  private listeners: Set<ToastListener> = new Set();

  subscribe(listener: ToastListener) {
    this.listeners.add(listener);
    listener(this.toasts);
    return () => {
      this.listeners.delete(listener);
    };
  }

  private notify() {
    const list = [...this.toasts];
    this.listeners.forEach((listener) => listener(list));
  }

  add(type: ToastType, message: string, description?: string, duration = 2000) {
    const id = Math.random().toString(36).substring(2, 9);
    const newToast: ToastData = { id, type, message, description, duration };
    // Giới hạn tối đa 5 thông báo cùng lúc trên màn hình
    this.toasts = [...this.toasts.slice(-4), newToast];
    this.notify();
    return id;
  }

  dismiss(id: string) {
    this.toasts = this.toasts.filter((t) => t.id !== id);
    this.notify();
  }

  success(message: string, description?: string, duration = 2000) {
    return this.add("success", message, description, duration);
  }

  error(message: string, description?: string, duration = 2000) {
    return this.add("error", message, description, duration);
  }

  warning(message: string, description?: string, duration = 2000) {
    return this.add("warning", message, description, duration);
  }

  info(message: string, description?: string, duration = 2000) {
    return this.add("info", message, description, duration);
  }
}

export const toast = new ToastManager();
