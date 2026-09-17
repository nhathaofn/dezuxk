import React, { useState } from "react";
import { models } from "@/../wailsjs/go/models";
import { StatusBadge } from "@/components/common/StatusBadge";
import { Button } from "@/components/ui/button";
import {
  ShieldCheck,
  RefreshCw,
  Trash2,
  CheckCircle2,
  XCircle,
  Clock,
  Sparkles,
  Zap,
  ExternalLink,
} from "lucide-react";

interface AccountCardProps {
  account: models.GoogleAccountResponse;
  onTest: (id: string) => Promise<models.AccountTestResult>;
  onRefresh: (id: string) => Promise<void>;
  onOpenBrowser: (id: string) => Promise<void>;
  onDelete: (id: string) => void;
}

export function AccountCard({
  account,
  onTest,
  onRefresh,
  onOpenBrowser,
  onDelete,
}: AccountCardProps) {
  const [isTesting, setIsTesting] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isOpeningBrowser, setIsOpeningBrowser] = useState(false);
  const [testResult, setTestResult] = useState<models.AccountTestResult | null>(null);

  const handleOpenBrowser = async () => {
    setIsOpeningBrowser(true);
    try {
      await onOpenBrowser(account.id);
    } finally {
      setIsOpeningBrowser(false);
    }
  };

  const handleTest = async () => {
    setIsTesting(true);
    setTestResult(null);
    try {
      const res = await onTest(account.id);
      setTestResult(res);
    } finally {
      setIsTesting(false);
    }
  };

  const handleRefresh = async () => {
    setIsRefreshing(true);
    try {
      await onRefresh(account.id);
      setTestResult(null);
    } finally {
      setIsRefreshing(false);
    }
  };

  const initial = (account.name || account.email || "G").charAt(0).toUpperCase();

  return (
    <div className="group relative flex flex-col xl:flex-row xl:items-center justify-between gap-4 rounded-2xl border bg-card p-4 sm:p-5 shadow-xs transition-all duration-200 hover:border-primary/40 hover:shadow-md">
      {/* Left & Middle Section: Account Info & Tokens (Horizontal Layout) */}
      <div className="flex flex-col md:flex-row md:items-center gap-4 flex-1 min-w-0">
        {/* Avatar & Basic Info */}
        <div className="flex items-center gap-3.5 min-w-0 shrink-0">
          {account.avatarUrl ? (
            <img
              src={account.avatarUrl}
              alt={account.name}
              className="size-11 shrink-0 rounded-full border border-border object-cover shadow-2xs"
            />
          ) : (
            <div className="flex size-11 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary font-bold text-sm shadow-2xs border border-primary/20">
              {initial}
            </div>
          )}

          <div className="min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h4 className="text-sm font-semibold text-foreground truncate">
                {account.name || "Google Account"}
              </h4>
              <StatusBadge status={account.status} />
            </div>
            <p className="text-xs text-muted-foreground truncate font-mono mt-0.5">
              {account.email}
            </p>
          </div>
        </div>

        {/* Divider on desktop */}
        <div className="hidden md:block h-10 w-px bg-border/60 shrink-0 mx-1" />

        {/* Tokens and Services Details */}
        <div className="flex flex-col gap-2 min-w-0 flex-1">
          {/* Services & Tokens pill row */}
          <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
            {/* Service badges */}
            <span className="inline-flex items-center gap-1 rounded-md bg-purple-500/10 px-2 py-0.5 font-medium text-purple-600 dark:text-purple-400 border border-purple-500/20">
              <Sparkles className="size-3" />
              gemini
            </span>
            <span className="inline-flex items-center gap-1 rounded-md bg-blue-500/10 px-2 py-0.5 font-medium text-blue-600 dark:text-blue-400 border border-blue-500/20">
              <Zap className="size-3" />
              flow
            </span>

            {/* Token status indicators */}
            <span className="inline-flex items-center gap-1 rounded-md bg-muted/60 px-2 py-0.5 text-muted-foreground border border-border/60">
              <CheckCircle2 className="size-3 text-emerald-500" />
              __Secure-1PSID
            </span>

            {account.hasPsidts ? (
              <span className="inline-flex items-center gap-1 rounded-md bg-emerald-500/10 px-2 py-0.5 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 font-medium">
                <CheckCircle2 className="size-3" />
                1PSIDTS (Quay vòng)
              </span>
            ) : (
              <span className="inline-flex items-center gap-1 rounded-md bg-amber-500/10 px-2 py-0.5 text-amber-600 dark:text-amber-400 border border-amber-500/20">
                Chờ 1PSIDTS
              </span>
            )}

            {account.hasSnlm0e && (
              <span className="inline-flex items-center gap-1 rounded-md bg-emerald-500/10 px-2 py-0.5 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 font-medium">
                <CheckCircle2 className="size-3" />
                CSRF Ready
              </span>
            )}

            {account.lastRefreshAt && (
              <span className="inline-flex items-center gap-1 text-[10px] text-muted-foreground ml-1">
                <Clock className="size-3 text-muted-foreground/70" />
                Làm mới: {account.lastRefreshAt}
              </span>
            )}
          </div>

          {/* Test result banner if available */}
          {testResult && (
            <div
              className={`rounded-lg border px-2.5 py-1.5 text-xs animate-in fade-in duration-150 ${
                testResult.success
                  ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
                  : "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300"
              }`}
            >
              <div className="flex items-center justify-between font-medium text-[11px]">
                <span className="flex items-center gap-1.5">
                  {testResult.success ? (
                    <CheckCircle2 className="size-3.5 text-emerald-500" />
                  ) : (
                    <XCircle className="size-3.5 text-red-500" />
                  )}
                  {testResult.message}
                </span>
                <span className="opacity-75 font-mono ml-2">{testResult.latencyMs}ms</span>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Right Section: Action Buttons in a Clean Horizontal Row */}
      <div className="flex items-center gap-2 shrink-0 pt-2 xl:pt-0 border-t xl:border-t-0 border-border/60">
        <Button
          variant="outline"
          size="sm"
          onClick={handleTest}
          disabled={isTesting || isRefreshing}
          className="h-8.5 gap-1.5 text-xs"
        >
          {isTesting ? (
            <span className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent" />
          ) : (
            <ShieldCheck className="size-3.5 text-primary" />
          )}
          Kiểm tra kết nối
        </Button>

        <Button
          variant="outline"
          size="sm"
          onClick={handleRefresh}
          disabled={isTesting || isRefreshing}
          className="h-8.5 gap-1.5 text-xs"
        >
          <RefreshCw className={`size-3.5 ${isRefreshing ? "animate-spin text-primary" : ""}`} />
          Làm mới phiên
        </Button>

        <Button
          variant="outline"
          size="sm"
          onClick={handleOpenBrowser}
          disabled={isOpeningBrowser}
          className="h-8.5 gap-1.5 text-xs font-medium text-foreground hover:border-primary/50"
          title="Mở cửa sổ Chrome độc lập gắn với profile tài khoản này"
        >
          <ExternalLink className={`size-3.5 ${isOpeningBrowser ? "animate-spin text-primary" : ""}`} />
          Mở Chrome
        </Button>

        <Button
          variant="ghost"
          size="sm"
          onClick={() => onDelete(account.id)}
          className="size-8.5 p-0 text-muted-foreground hover:bg-red-500/10 hover:text-red-500 rounded-lg transition-colors ml-1"
          title="Xóa tài khoản"
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
    </div>
  );
}
