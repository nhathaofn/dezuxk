import React, { useState } from "react";
import { models } from "@/../wailsjs/go/models";
import { ToggleSwitch } from "./ToggleSwitch";
import {
  RefreshCw,
  Globe,
  Trash2,
  Loader2,
  ExternalLink,
  ShieldCheck,
  Copy,
  Check,
  Clock,
  Zap,
  Sparkles,
} from "lucide-react";

interface AccountTableRowProps {
  index: number;
  account: models.GoogleAccountResponse;
  isActive: boolean;
  proxyValue?: string;
  liveMetrics?: models.LiveAccountMetrics;
  onToggleActive: (id: string, active: boolean) => void;
  onRefresh: (id: string) => Promise<void>;
  onTest: (id: string) => Promise<void>;
  onOpenProxyModal: (account: models.GoogleAccountResponse) => void;
  onOpenBrowser: (id: string) => Promise<void>;
  onDelete: (id: string) => void;
  isRefreshing?: boolean;
}

function formatSyncTime(dateStr?: string): string {
  if (!dateStr) return "";
  try {
    const isoStr = dateStr.includes("T")
      ? dateStr.endsWith("Z") ? dateStr : `${dateStr}Z`
      : `${dateStr.replace(" ", "T")}Z`;
    const d = new Date(isoStr);
    if (!isNaN(d.getTime())) {
      return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
    }
  } catch {}
  return dateStr.slice(11, 16);
}

export function AccountTableRow({
  index,
  account,
  isActive,
  proxyValue = "-",
  liveMetrics,
  onToggleActive,
  onRefresh,
  onTest,
  onOpenProxyModal,
  onOpenBrowser,
  onDelete,
  isRefreshing: isRefreshingProp = false,
}: AccountTableRowProps) {
  const [isLocalRefreshing, setIsLocalRefreshing] = useState(false);
  const isRefreshing = isRefreshingProp || isLocalRefreshing;
  const [isOpeningBrowser, setIsOpeningBrowser] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [copiedEmail, setCopiedEmail] = useState(false);

  const handleRefresh = async () => {
    setIsLocalRefreshing(true);
    try {
      await onRefresh(account.id);
    } finally {
      setIsLocalRefreshing(false);
    }
  };

  const handleTest = async () => {
    setIsTesting(true);
    try {
      await onTest(account.id);
    } finally {
      setIsTesting(false);
    }
  };

  const handleOpenBrowser = async () => {
    setIsOpeningBrowser(true);
    try {
      await onOpenBrowser(account.id);
    } finally {
      setIsOpeningBrowser(false);
    }
  };

  const handleCopyEmail = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(account.email);
    setCopiedEmail(true);
    setTimeout(() => setCopiedEmail(false), 1500);
  };

  // Status Badge UI
  let statusBadge = (
    <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-semibold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/25 shadow-2xs whitespace-nowrap">
      <span className="relative flex size-2 shrink-0">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75"></span>
        <span className="relative inline-flex size-2 rounded-full bg-emerald-500"></span>
      </span>
      HOẠT ĐỘNG
    </span>
  );

  if (account.status === "PENDING") {
    statusBadge = (
      <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-semibold bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/25 shadow-2xs whitespace-nowrap">
        <span className="size-2 rounded-full bg-amber-500 shrink-0" />
        CHỜ KIỂM TRA
      </span>
    );
  } else if (!isActive || account.status === "DISABLED") {
    statusBadge = (
      <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium bg-muted text-muted-foreground border border-border shadow-2xs whitespace-nowrap">
        <span className="size-2 rounded-full bg-muted-foreground/40 shrink-0" />
        ĐÃ TẮT
      </span>
    );
  } else if (account.status === "EXPIRED" || account.status === "ERROR") {
    statusBadge = (
      <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-semibold bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/25 shadow-2xs whitespace-nowrap">
        <span className="size-2 rounded-full bg-rose-500 shrink-0" />
        HẾT HẠN
      </span>
    );
  } else if (account.status === "REFRESHING" || isRefreshing) {
    statusBadge = (
      <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-semibold bg-sky-500/10 text-sky-600 dark:text-sky-400 border border-sky-500/25 shadow-2xs whitespace-nowrap">
        <Loader2 className="size-3 animate-spin text-sky-500 shrink-0" />
        LÀM MỚI
      </span>
    );
  }

  const hasProxy = Boolean(proxyValue && proxyValue !== "-");
  const canRefresh = account.status !== "PENDING";
  const flow = liveMetrics?.flow;
  const gemini = liveMetrics?.gemini;
  const displayTier = flow?.tier || gemini?.tier || account.tier || "FREE";
  const liveTime = liveMetrics?.retrievedAt ? formatSyncTime(liveMetrics.retrievedAt) : "";
  const flowTotal = flow?.hasTotalCredits ? flow.totalCredits : 0;
  const flowBuckets = flow
    ? `Ngày: ${flow.hasDailyCredits ? flow.dailyCredits.toLocaleString() : "—"} · Tháng: ${flow.hasMonthlyCredits ? flow.monthlyCredits.toLocaleString() : "—"}`
    : "Chưa đồng bộ";
  const geminiCurrent = gemini?.hasCurrentRemaining ? `${gemini.currentRemainingPercent}% còn` : "—";
  const geminiWeekly = gemini?.hasWeeklyRemaining ? `${gemini.weeklyRemainingPercent}%/tuần` : "—";
  const geminiResets = [gemini?.currentResetAt, gemini?.weeklyResetAt].filter(Boolean).join(" · ");
  const flowResets = [flow?.dailyResetAt, flow?.monthlyResetAt].filter(Boolean).join(" · ");

  return (
    <tr className="border-b border-border/50 hover:bg-muted/40 transition-colors duration-150 group/row">
      {/* 1. STT */}
      <td className="py-2.5 px-1 text-center text-xs font-mono font-medium text-muted-foreground/80 select-none">
        {index}
      </td>

      {/* 2. TẮT/BẬT */}
      <td className="py-2.5 px-1 text-center align-middle">
        <div className="flex justify-center items-center">
          <ToggleSwitch
            size="sm"
            checked={isActive}
            onChange={(checked) => onToggleActive(account.id, checked)}
          />
        </div>
      </td>

      {/* 3. Tài khoản Google (Email & Avatar) */}
      <td className="py-2.5 px-2 min-w-0 align-middle">
        <div className="flex items-center gap-2">
          <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-primary/15 via-primary/10 to-transparent border border-primary/20 text-primary text-[11px] font-bold uppercase select-none shadow-2xs">
            {account.email.charAt(0) || "G"}
          </div>
          <div className="flex flex-col min-w-0 leading-tight">
            <div className="flex items-center gap-1 group/email">
              <span
                className="text-xs font-semibold text-foreground truncate select-all"
                title={account.email}
              >
                {account.email}
              </span>
              <button
                type="button"
                onClick={handleCopyEmail}
                className="opacity-0 group-hover/email:opacity-100 p-0.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-all cursor-pointer"
                title={copiedEmail ? "Đã sao chép!" : "Sao chép email"}
              >
                {copiedEmail ? (
                  <Check className="size-3 text-emerald-500" />
                ) : (
                  <Copy className="size-3" />
                )}
              </button>
            </div>
            {account.lastRefreshAt && (
              <span className="flex items-center gap-1 text-[10px] text-muted-foreground truncate mt-0.5">
                <Clock className="size-2.5 shrink-0 opacity-70" />
                <span>{formatSyncTime(account.lastRefreshAt)}</span>
              </span>
            )}
          </div>
        </div>
      </td>

      {/* 4. Gói / Hạng */}
      <td className="py-2.5 px-1 text-center align-middle">
        <span
          className={`inline-flex items-center px-1.5 py-0.5 rounded text-[9px] font-bold tracking-wider uppercase border shadow-2xs whitespace-nowrap ${
            displayTier === "PRO" || displayTier === "PLUS" || displayTier === "ULTRA"
              ? "bg-violet-500/10 text-violet-600 dark:text-violet-400 border-violet-500/25"
              : "bg-muted/70 text-muted-foreground border-border"
          }`}
        >
          {displayTier}
        </span>
      </td>

      {/* 5. Quota snapshot (Flow + Gemini) */}
      <td className="py-2 px-1.5 text-left align-middle">
        <div className="flex flex-col gap-1 w-full">
          {/* Flow Quota Pill */}
          <div
            className="flex items-center justify-between gap-1.5 px-2 py-0.5 rounded-md bg-sky-500/5 dark:bg-sky-500/10 border border-sky-500/15 text-[10px] transition-colors hover:bg-sky-500/10"
            title={`Google Flow Credits:\nTổng: ${flowTotal.toLocaleString()}\nChi tiết: ${flowBuckets}${flowResets ? `\nReset: ${flowResets}` : ""}`}
          >
            <div className="flex items-center gap-1 font-semibold text-sky-600 dark:text-sky-400">
              <Zap className="size-2.5 shrink-0" />
              <span>Flow</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="font-mono font-bold text-foreground">
                {flowTotal.toLocaleString()}
              </span>
              <span className="text-[9px] text-muted-foreground">cr</span>
            </div>
          </div>

          {/* Gemini Quota Pill */}
          <div
            className="flex items-center justify-between gap-1.5 px-2 py-0.5 rounded-md bg-violet-500/5 dark:bg-violet-500/10 border border-violet-500/15 text-[10px] transition-colors hover:bg-violet-500/10"
            title={`Google Gemini AI:\nHiện tại: ${geminiCurrent}\nTheo tuần: ${geminiWeekly}${geminiResets ? `\nReset: ${geminiResets}` : ""}`}
          >
            <div className="flex items-center gap-1 font-semibold text-violet-600 dark:text-violet-400">
              <Sparkles className="size-2.5 shrink-0" />
              <span>Gemini</span>
            </div>
            <div className="flex items-center gap-1">
              <span className="font-mono font-bold text-foreground">
                {geminiCurrent}
              </span>
              <span className="text-[9px] text-muted-foreground/80 font-mono">{geminiWeekly}</span>
            </div>
          </div>

          {/* Explicit quota-sync metadata */}
          <div className="flex items-center justify-between text-[9px] text-muted-foreground/70 px-0.5">
            <span className="flex items-center gap-1">
              <span className={`size-1.5 rounded-full ${liveMetrics?.status === "LIVE" ? "bg-emerald-500" : "bg-muted-foreground/40"}`} />
              <span>{liveMetrics ? (liveMetrics.status === "LIVE" ? "Đã đồng bộ" : liveMetrics.status) : "Chưa đồng bộ"}</span>
            </span>
            {liveTime && <span className="font-mono">{liveTime}</span>}
          </div>
        </div>
      </td>

      {/* 6. Proxy */}
      <td className="py-2.5 px-1 text-center align-middle">
        <button
          type="button"
          onClick={() => onOpenProxyModal(account)}
          className={`group inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[10px] font-mono transition-all duration-150 cursor-pointer max-w-[85px] ${
            hasProxy
              ? "bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/25 hover:bg-amber-500/20 shadow-2xs"
              : "bg-muted/40 text-muted-foreground border border-border/60 hover:bg-muted hover:text-foreground"
          }`}
          title={hasProxy ? `Proxy: ${proxyValue} (Bấm để thay đổi)` : "Kết nối trực tiếp (Bấm để cấu hình Proxy)"}
        >
          <Globe className="size-3 shrink-0 opacity-80 group-hover:opacity-100" />
          <span className="truncate">{hasProxy ? proxyValue.replace(/^https?:\/\//, "") : "Direct"}</span>
        </button>
      </td>

      {/* 7. Trạng thái */}
      <td className="py-2.5 px-1 text-center align-middle">
        {statusBadge}
      </td>

      {/* 8. Thao tác */}
      <td className="py-2.5 px-1 text-center align-middle">
        <div className="flex items-center justify-center gap-1">
          {/* Nút 1: Làm mới token headless */}
          <button
            type="button"
            onClick={handleRefresh}
            disabled={isRefreshing || !canRefresh}
            className="flex size-6.5 items-center justify-center rounded-md border border-border/70 bg-background hover:bg-emerald-500/10 hover:text-emerald-600 dark:hover:text-emerald-400 hover:border-emerald-500/30 text-muted-foreground transition-all duration-150 active:scale-95 disabled:opacity-40 cursor-pointer shadow-2xs"
            title={canRefresh ? "Làm mới token qua Chrome headless" : "Hãy kiểm tra phiên trước khi làm mới"}
          >
            {isRefreshing ? (
              <Loader2 className="size-3 animate-spin text-emerald-500" />
            ) : (
              <RefreshCw className="size-3" />
            )}
          </button>

          {/* Nút 2: Kiểm tra phiên & kết nối */}
          <button
            type="button"
            onClick={handleTest}
            disabled={isTesting}
            className="flex size-6.5 items-center justify-center rounded-md border border-border/70 bg-background hover:bg-violet-500/10 hover:text-violet-600 dark:hover:text-violet-400 hover:border-violet-500/30 text-muted-foreground transition-all duration-150 active:scale-95 disabled:opacity-40 cursor-pointer shadow-2xs"
            title="Kiểm tra kết nối và trạng thái phiên đăng nhập"
          >
            {isTesting ? (
              <Loader2 className="size-3 animate-spin text-violet-500" />
            ) : (
              <ShieldCheck className="size-3" />
            )}
          </button>

          {/* Nút 3: Cấu hình Proxy riêng */}
          <button
            type="button"
            onClick={() => onOpenProxyModal(account)}
            className="flex size-6.5 items-center justify-center rounded-md border border-border/70 bg-background hover:bg-amber-500/10 hover:text-amber-600 dark:hover:text-amber-400 hover:border-amber-500/30 text-muted-foreground transition-all duration-150 active:scale-95 cursor-pointer shadow-2xs"
            title="Cấu hình Proxy riêng cho tài khoản"
          >
            <Globe className="size-3" />
          </button>

          {/* Nút 4: Mở Chrome độc lập */}
          <button
            type="button"
            onClick={handleOpenBrowser}
            disabled={isOpeningBrowser}
            className="flex size-6.5 items-center justify-center rounded-md border border-border/70 bg-background hover:bg-sky-500/10 hover:text-sky-600 dark:hover:text-sky-400 hover:border-sky-500/30 text-muted-foreground transition-all duration-150 active:scale-95 disabled:opacity-40 cursor-pointer shadow-2xs"
            title="Mở trình duyệt Google Chrome thật của tài khoản"
          >
            {isOpeningBrowser ? (
              <Loader2 className="size-3 animate-spin text-sky-500" />
            ) : (
              <ExternalLink className="size-3" />
            )}
          </button>

          {/* Nút 5: Xóa tài khoản */}
          <button
            type="button"
            onClick={() => onDelete(account.id)}
            className="flex size-6.5 items-center justify-center rounded-md border border-border/70 bg-background hover:bg-rose-500/10 hover:text-rose-600 dark:hover:text-rose-400 hover:border-rose-500/30 text-muted-foreground transition-all duration-150 active:scale-95 cursor-pointer shadow-2xs"
            title="Xóa tài khoản và dữ liệu profile"
          >
            <Trash2 className="size-3" />
          </button>
        </div>
      </td>
    </tr>
  );
}
