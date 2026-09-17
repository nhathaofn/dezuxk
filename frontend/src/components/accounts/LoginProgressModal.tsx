import React from "react";
import { Button } from "@/components/ui/button";
import {
  Globe,
  KeyRound,
  CheckCircle2,
  XCircle,
  Clock,
  ShieldCheck,
  AlertCircle,
} from "lucide-react";

interface LoginProgressModalProps {
  isOpen: boolean;
  step: string; // INITIALIZING | WAITING_USER_LOGIN | EXTRACTING_COOKIES | COMPLETED | FAILED | CANCELLED
  message: string;
  error?: string;
  onCancel: () => void;
  onClose: () => void;
}

export function LoginProgressModal({
  isOpen,
  step,
  message,
  error,
  onCancel,
  onClose,
}: LoginProgressModalProps) {
  if (!isOpen) return null;

  const isCompleted = step === "COMPLETED";
  const isFailed = step === "FAILED" || step === "CANCELLED";

  const stepsInfo = [
    {
      id: "init",
      title: "1. Khởi động Chrome thật",
      desc: "Mở trình duyệt Google Chrome chính chủ trên máy với profile cô lập",
      isCurrent: step === "INITIALIZING",
      isDone:
        step === "WAITING_USER_LOGIN" ||
        step === "EXTRACTING_COOKIES" ||
        step === "COMPLETED",
      icon: Globe,
    },
    {
      id: "login",
      title: "2. Đăng nhập Google & 2FA",
      desc: "Nhập tài khoản trên cửa sổ Chrome vừa xuất hiện (Google an toàn 100%)",
      isCurrent: step === "WAITING_USER_LOGIN",
      isDone: step === "EXTRACTING_COOKIES" || step === "COMPLETED",
      icon: KeyRound,
    },
    {
      id: "extract",
      title: "3. Tự động trích xuất Tokens & CSRF",
      desc: "Lắng nghe qua Chrome DevTools: bắt __Secure-1PSID và SNlM0e",
      isCurrent: step === "EXTRACTING_COOKIES",
      isDone: step === "COMPLETED",
      icon: ShieldCheck,
    },
    {
      id: "done",
      title: "4. Hoàn tất & Kích hoạt",
      desc: "Đóng Chrome, lưu vào cơ sở dữ liệu và duy trì session lâu dài",
      isCurrent: step === "COMPLETED",
      isDone: step === "COMPLETED",
      icon: CheckCircle2,
    },
  ];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-200">
      <div className="relative w-full max-w-lg rounded-2xl border bg-card p-6 shadow-2xl text-card-foreground animate-in zoom-in-95 duration-200">
        {/* Header */}
        <div className="flex items-center gap-3 border-b border-border/60 pb-4">
          <div className="flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <Globe className="size-5" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-foreground">
              Đăng nhập Google qua Chrome thật
            </h2>
            <p className="text-xs text-muted-foreground">
              Mở cửa sổ Chrome độc lập, an toàn và chống chặn bot
            </p>
          </div>
        </div>

        {/* Live Step Tracker */}
        <div className="my-6 space-y-3">
          {stepsInfo.map((s, idx) => {
            const Icon = s.icon;
            return (
              <div
                key={s.id}
                className={`flex items-start gap-3 rounded-xl border p-3 transition-all duration-200 ${
                  s.isDone
                    ? "border-emerald-500/30 bg-emerald-500/5 text-foreground"
                    : s.isCurrent
                    ? "border-primary/40 bg-primary/5 text-foreground shadow-xs"
                    : "border-border/40 bg-muted/20 text-muted-foreground opacity-60"
                }`}
              >
                <div className="mt-0.5 shrink-0">
                  {s.isDone ? (
                    <div className="flex size-6 items-center justify-center rounded-full bg-emerald-500 text-white shadow-xs">
                      <CheckCircle2 className="size-3.5" />
                    </div>
                  ) : s.isCurrent ? (
                    <div className="flex size-6 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-xs">
                      <span className="size-3 animate-spin rounded-full border-2 border-primary-foreground border-t-transparent" />
                    </div>
                  ) : (
                    <div className="flex size-6 items-center justify-center rounded-full border border-border bg-background text-[11px] font-semibold">
                      {idx + 1}
                    </div>
                  )}
                </div>

                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-semibold">{s.title}</span>
                    {s.isCurrent && (
                      <span className="inline-flex items-center rounded-full bg-primary/20 px-2 py-0.2 text-[10px] font-medium text-primary animate-pulse">
                        Đang xử lý
                      </span>
                    )}
                  </div>
                  <p className="mt-0.5 text-[11px] text-muted-foreground leading-normal">
                    {s.desc}
                  </p>
                </div>
              </div>
            );
          })}
        </div>

        {/* Current status message box */}
        <div
          className={`rounded-xl border p-3.5 text-xs ${
            isCompleted
              ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
              : isFailed
              ? "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300"
              : "border-primary/20 bg-primary/5 text-foreground"
          }`}
        >
          <div className="flex items-center gap-2 font-medium">
            {isCompleted ? (
              <CheckCircle2 className="size-4 shrink-0 text-emerald-500" />
            ) : isFailed ? (
              <AlertCircle className="size-4 shrink-0 text-red-500" />
            ) : (
              <Clock className="size-4 shrink-0 animate-pulse text-primary" />
            )}
            <span>{message || "Đang kết nối..."}</span>
          </div>
          {error && <p className="mt-1 text-[11px] opacity-85">{error}</p>}
        </div>

        {/* Action Buttons */}
        <div className="mt-6 flex items-center justify-end gap-3">
          {isCompleted ? (
            <Button onClick={onClose} className="w-full sm:w-auto">
              Hoàn tất
            </Button>
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={onCancel}
              className="text-muted-foreground hover:text-foreground"
            >
              Hủy bỏ tiến trình
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
