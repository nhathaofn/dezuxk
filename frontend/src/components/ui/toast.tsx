import React, { useEffect, useState, useRef } from "react";
import { toast, type ToastData } from "@/lib/toast";
import { cn } from "@/lib/utils";
import { CheckCircle2, AlertCircle, AlertTriangle, Info, X } from "lucide-react";

interface ToastItemProps {
  item: ToastData;
}

function ToastItem({ item }: ToastItemProps) {
  const [isHovered, setIsHovered] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const startTimer = (ms: number) => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      toast.dismiss(item.id);
    }, ms);
  };

  useEffect(() => {
    // Bắt đầu đếm ngược 2s tắt thông báo khi không có chuột di vào
    if (!isHovered) {
      startTimer(item.duration || 2000);
    }

    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [isHovered, item.duration, item.id]);

  const handleMouseEnter = () => {
    // Khi di chuột vào thì dừng đếm ngược (không tắt)
    setIsHovered(true);
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  };

  const handleMouseLeave = () => {
    // Khi chuột rời ra thì bắt đầu đếm lại 2s là tắt
    setIsHovered(false);
  };

  const typeStyles = {
    success: {
      border: "border-emerald-500/30 dark:border-emerald-500/40",
      iconBg: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
      progressBg: "bg-emerald-500",
      icon: <CheckCircle2 className="size-4 shrink-0" />,
    },
    error: {
      border: "border-red-500/30 dark:border-red-500/40",
      iconBg: "bg-red-500/10 text-red-600 dark:text-red-400",
      progressBg: "bg-red-500",
      icon: <AlertCircle className="size-4 shrink-0" />,
    },
    warning: {
      border: "border-amber-500/30 dark:border-amber-500/40",
      iconBg: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
      progressBg: "bg-amber-500",
      icon: <AlertTriangle className="size-4 shrink-0" />,
    },
    info: {
      border: "border-blue-500/30 dark:border-blue-500/40",
      iconBg: "bg-blue-500/10 text-blue-600 dark:text-blue-400",
      progressBg: "bg-blue-500",
      icon: <Info className="size-4 shrink-0" />,
    },
  };

  const currentType = typeStyles[item.type] || typeStyles.info;

  return (
    <div
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
      role="alert"
      className={cn(
        "pointer-events-auto relative flex w-full items-center gap-3 overflow-hidden rounded-xl border bg-card/95 p-3.5 text-card-foreground shadow-lg backdrop-blur-md transition-all duration-200 select-none",
        "animate-in slide-in-from-bottom-5 fade-in duration-200",
        currentType.border
      )}
    >
      {/* Icon theo loại thông báo */}
      <div
        className={cn(
          "flex size-7 shrink-0 items-center justify-center rounded-lg",
          currentType.iconBg
        )}
      >
        {currentType.icon}
      </div>

      {/* Nội dung thông báo */}
      <div className="flex-1 pr-1 text-left">
        <p className="text-xs font-semibold text-foreground leading-tight">
          {item.message}
        </p>
        {item.description && (
          <p className="mt-1 text-[11px] text-muted-foreground leading-normal">
            {item.description}
          </p>
        )}
      </div>

      {/* Nút đóng nhanh - Canh giữa theo chiều dọc */}
      <button
        type="button"
        onClick={() => toast.dismiss(item.id)}
        aria-label="Đóng thông báo"
        className="shrink-0 self-center rounded-md p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
      >
        <X className="size-3.5" />
      </button>

      {/* Thanh đo thời gian tự tắt 2s (tạm dừng khi hover chuột) */}
      <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-muted/40 overflow-hidden">
        <div
          className={cn("h-full transition-all", currentType.progressBg)}
          style={{
            width: isHovered ? "100%" : "0%",
            transitionDuration: isHovered ? "0ms" : `${item.duration || 2000}ms`,
            transitionTimingFunction: "linear",
          }}
        />
      </div>
    </div>
  );
}

export function ToastContainer() {
  const [toasts, setToasts] = useState<ToastData[]>([]);

  useEffect(() => {
    return toast.subscribe((updatedToasts) => {
      setToasts(updatedToasts);
    });
  }, []);

  if (toasts.length === 0) return null;

  return (
    <aside
      aria-label="Thông báo hệ thống"
      className="fixed bottom-5 right-5 z-50 flex flex-col gap-2.5 max-w-sm w-full pointer-events-none"
    >
      {toasts.map((t) => (
        <ToastItem key={t.id} item={t} />
      ))}
    </aside>
  );
}
