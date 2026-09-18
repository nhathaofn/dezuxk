import React, { useState } from "react";
import { models } from "@/../wailsjs/go/models";
import { ToggleSwitch } from "./ToggleSwitch";
import { RefreshCw, Globe, Trash2, Loader2, ExternalLink, ShieldCheck, Check, X, Edit2 } from "lucide-react";

interface AccountTableRowProps {
  index: number;
  account: models.GoogleAccountResponse;
  isActive: boolean;
  proxyValue?: string;
  credit?: number;
  onToggleActive: (id: string, active: boolean) => void;
  onRefresh: (id: string) => Promise<void>;
  onTest: (id: string) => Promise<void>;
  onOpenProxyModal: (account: models.GoogleAccountResponse) => void;
  onOpenBrowser: (id: string) => Promise<void>;
  onDelete: (id: string) => void;
  onUpdateCredits?: (id: string, credits: number) => Promise<void>;
  isRefreshing?: boolean;
}

function formatSyncTime(dateStr?: string): string {
  if (!dateStr) return "";
  try {
    // Backend lưu thời gian dạng UTC ("YYYY-MM-DD HH:mm:ss"), cần chuẩn hóa để JS nhận diện là UTC rồi đổi sang giờ máy cục bộ
    const isoStr = dateStr.includes("T")
      ? (dateStr.endsWith("Z") ? dateStr : `${dateStr}Z`)
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
  credit,
  onToggleActive,
  onRefresh,
  onTest,
  onOpenProxyModal,
  onOpenBrowser,
  onDelete,
  onUpdateCredits,
  isRefreshing: isRefreshingProp = false,
}: AccountTableRowProps) {
  const [isLocalRefreshing, setIsLocalRefreshing] = useState(false);
  const isRefreshing = isRefreshingProp || isLocalRefreshing;
  const [isOpeningBrowser, setIsOpeningBrowser] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [isEditingCredit, setIsEditingCredit] = useState(false);
  const [editCreditValue, setEditCreditValue] = useState("");
  const [isSavingCredit, setIsSavingCredit] = useState(false);

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

  const handleSaveCredit = async () => {
    const val = parseInt(editCreditValue, 10);
    if (isNaN(val) || val < 0) {
      setIsEditingCredit(false);
      return;
    }
    setIsSavingCredit(true);
    try {
      if (onUpdateCredits) {
        await onUpdateCredits(account.id, val);
      }
      setIsEditingCredit(false);
    } catch (err) {
      console.error(err);
    } finally {
      setIsSavingCredit(false);
    }
  };

  // Real Status Badge UI
  let statusBadge = (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 whitespace-nowrap">
      <span className="size-1.5 rounded-full bg-emerald-500 shrink-0" />
      HOẠT ĐỘNG
    </span>
  );

  if (!isActive || account.status === "DISABLED") {
    statusBadge = (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-muted text-muted-foreground border border-border whitespace-nowrap">
        <span className="size-1.5 rounded-full bg-muted-foreground/50 shrink-0" />
        ĐÃ TẮT
      </span>
    );
  } else if (account.status === "EXPIRED" || account.status === "ERROR") {
    statusBadge = (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20 whitespace-nowrap">
        <span className="size-1.5 rounded-full bg-rose-500 shrink-0" />
        HẾT HẠN
      </span>
    );
  } else if (account.status === "REFRESHING" || isRefreshing) {
    statusBadge = (
      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20 whitespace-nowrap">
        <Loader2 className="size-3 animate-spin text-amber-500 shrink-0" />
        LÀM MỚI
      </span>
    );
  }

  // Dynamic real credits from database
  const realCredits = typeof account.credits === "number" ? account.credits : (typeof credit === "number" ? credit : 0);
  const displayCredits = realCredits.toLocaleString();
  const displayTier = account.tier || "FREE";
  const hasProxy = proxyValue && proxyValue !== "-";

  return (
    <tr className="border-b border-border/50 hover:bg-muted/40 transition-colors duration-100">
      {/* 1. STT */}
      <td className="py-1.5 px-1 text-center text-xs font-medium text-muted-foreground select-none">
        {index}
      </td>

      {/* 2. TẮT/BẬT */}
      <td className="py-1.5 px-0.5 text-center">
        <div className="flex justify-center items-center">
          <ToggleSwitch
            size="sm"
            checked={isActive}
            onChange={(checked) => onToggleActive(account.id, checked)}
          />
        </div>
      </td>

      {/* 3. Tài khoản Email (Real info) */}
      <td className="py-1.5 px-2 min-w-0">
        <div className="flex items-center gap-2">
          <div className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary text-[9px] font-bold uppercase select-none">
            {account.email.charAt(0) || "G"}
          </div>
          <div className="flex flex-col min-w-0 leading-tight">
            <span
              className="text-xs font-medium text-foreground truncate select-all"
              title={account.email}
            >
              {account.email}
            </span>
            {account.lastRefreshAt && (
              <span className="text-[10px] text-muted-foreground truncate leading-none mt-0.5">
                Đồng bộ: {formatSyncTime(account.lastRefreshAt)}
              </span>
            )}
          </div>
        </div>
      </td>

      {/* 4. Loại Hạng (PRO) */}
      <td className="py-1.5 px-0.5 text-center">
        <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-bold tracking-wide uppercase bg-violet-500/10 text-violet-600 dark:text-violet-400 border border-violet-500/20 whitespace-nowrap">
          {displayTier}
        </span>
      </td>

      {/* 5. Tín dụng (Credits từ database - Cho phép chỉnh sửa động) */}
      <td className="py-1.5 px-1 text-center whitespace-nowrap">
        {isEditingCredit ? (
          <div className="inline-flex items-center justify-center gap-1">
            <input
              type="number"
              min="0"
              value={editCreditValue}
              onChange={(e) => setEditCreditValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleSaveCredit();
                if (e.key === "Escape") setIsEditingCredit(false);
              }}
              autoFocus
              className="w-16 px-1 py-0.5 text-xs text-center font-mono font-semibold bg-background border border-primary rounded focus:outline-none focus:ring-1 focus:ring-primary text-foreground"
            />
            <button
              type="button"
              onClick={handleSaveCredit}
              disabled={isSavingCredit}
              className="flex size-4 items-center justify-center rounded bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20 cursor-pointer"
              title="Lưu số tín dụng"
            >
              {isSavingCredit ? <Loader2 className="size-2.5 animate-spin" /> : <Check className="size-2.5" />}
            </button>
            <button
              type="button"
              onClick={() => setIsEditingCredit(false)}
              className="flex size-4 items-center justify-center rounded bg-muted text-muted-foreground hover:bg-muted/80 cursor-pointer"
              title="Hủy"
            >
              <X className="size-2.5" />
            </button>
          </div>
        ) : (
          <div
            className="group/cred inline-flex items-center justify-center gap-1 cursor-pointer py-0.5 px-1.5 rounded hover:bg-muted/60 transition-colors"
            onClick={() => {
              setEditCreditValue(String(realCredits));
              setIsEditingCredit(true);
            }}
            title="Nhấp đúp hoặc bấm để sửa số Credit thực tế"
          >
            {isRefreshing && <Loader2 className="size-2.5 animate-spin text-amber-500" />}
            <span className="text-xs font-semibold text-foreground font-mono">
              {displayCredits}
            </span>
            <Edit2 className="size-2.5 text-muted-foreground opacity-0 group-hover/cred:opacity-100 transition-opacity" />
          </div>
        )}
      </td>

      {/* 6. Proxy */}
      <td className="py-1.5 px-0.5 text-center">
        <button
          type="button"
          onClick={() => onOpenProxyModal(account)}
          className={`inline-flex items-center justify-center gap-1 px-1.5 py-0.5 rounded text-[10px] transition-colors ${
            hasProxy
              ? "bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20 font-medium"
              : "text-muted-foreground hover:bg-muted"
          }`}
          title={hasProxy ? `Proxy: ${proxyValue}` : "Chưa cài proxy (nhấp để cấu hình)"}
        >
          <Globe className="size-3 shrink-0" />
          <span className="truncate max-w-[60px]">{hasProxy ? proxyValue : "-"}</span>
        </button>
      </td>

      {/* 7. Trạng thái */}
      <td className="py-1.5 px-1 text-center">
        {statusBadge}
      </td>

      {/* 8. Hành động */}
      <td className="py-1.5 px-1 text-center">
        <div className="flex items-center justify-center gap-1">
          {/* Nút 1: Làm mới token headless */}
          <button
            type="button"
            onClick={handleRefresh}
            disabled={isRefreshing}
            className="flex size-6 items-center justify-center rounded-md bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 transition-all active:scale-95 disabled:opacity-50 cursor-pointer"
            title="Làm mới token qua Chrome headless"
          >
            {isRefreshing ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              <RefreshCw className="size-3" />
            )}
          </button>

          {/* Nút: Kiểm tra kết nối */}
          <button
            type="button"
            onClick={handleTest}
            disabled={isTesting}
            className="flex size-6 items-center justify-center rounded-md bg-purple-500/10 hover:bg-purple-500/20 text-purple-600 dark:text-purple-400 border border-purple-500/20 transition-all active:scale-95 disabled:opacity-50 cursor-pointer"
            title="Kiểm tra kết nối và trạng thái phiên đăng nhập"
          >
            {isTesting ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              <ShieldCheck className="size-3" />
            )}
          </button>

          {/* Nút 2: Cài đặt Proxy */}
          <button
            type="button"
            onClick={() => onOpenProxyModal(account)}
            className="flex size-6 items-center justify-center rounded-md bg-amber-500/10 hover:bg-amber-500/20 text-amber-600 dark:text-amber-400 border border-amber-500/20 transition-all active:scale-95 cursor-pointer text-[10px] font-bold"
            title="Cài đặt Proxy riêng"
          >
            P
          </button>

          {/* Nút 3: Mở cửa sổ Chrome thật */}
          <button
            type="button"
            onClick={handleOpenBrowser}
            disabled={isOpeningBrowser}
            className="flex size-6 items-center justify-center rounded-md bg-blue-500/10 hover:bg-blue-500/20 text-blue-600 dark:text-blue-400 border border-blue-500/20 transition-all active:scale-95 disabled:opacity-50 cursor-pointer"
            title="Mở trình duyệt Chrome thật của tài khoản"
          >
            {isOpeningBrowser ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              <ExternalLink className="size-3" />
            )}
          </button>

          {/* Nút 4: Xóa tài khoản */}
          <button
            type="button"
            onClick={() => onDelete(account.id)}
            className="flex size-6 items-center justify-center rounded-md bg-rose-500/10 hover:bg-rose-500/20 text-rose-600 dark:text-rose-400 border border-rose-500/20 transition-all active:scale-95 cursor-pointer"
            title="Xóa tài khoản và dữ liệu profile"
          >
            <Trash2 className="size-3" />
          </button>
        </div>
      </td>
    </tr>
  );
}
