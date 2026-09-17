import React from "react";
import { cn } from "@/lib/utils";
import { CheckCircle2, RefreshCw, AlertTriangle, XCircle } from "lucide-react";

interface StatusBadgeProps {
  status: string;
  className?: string;
  showIcon?: boolean;
}

export function StatusBadge({ status, className, showIcon = true }: StatusBadgeProps) {
  const normalized = (status || "").toUpperCase();

  switch (normalized) {
    case "ACTIVE":
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold select-none",
            "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30",
            className
          )}
        >
          {showIcon && (
            <span className="relative flex size-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
              <span className="relative inline-flex size-2 rounded-full bg-emerald-500" />
            </span>
          )}
          Hoạt động
        </span>
      );

    case "REFRESHING":
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold select-none",
            "bg-blue-500/15 text-blue-600 dark:text-blue-400 border border-blue-500/30",
            className
          )}
        >
          {showIcon && <RefreshCw className="size-3 animate-spin" />}
          Đang làm mới
        </span>
      );

    case "EXPIRED":
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold select-none",
            "bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30",
            className
          )}
        >
          {showIcon && <AlertTriangle className="size-3" />}
          Hết hạn
        </span>
      );

    case "ERROR":
    case "FAILED":
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold select-none",
            "bg-red-500/15 text-red-600 dark:text-red-400 border border-red-500/30",
            className
          )}
        >
          {showIcon && <XCircle className="size-3" />}
          Lỗi
        </span>
      );

    default:
      return (
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium select-none",
            "bg-muted text-muted-foreground border border-border",
            className
          )}
        >
          {status || "Không xác định"}
        </span>
      );
  }
}
