import React, { useState } from "react";
import { models } from "@/../wailsjs/go/models";
import { ToggleSwitch } from "./ToggleSwitch";
import { Check, RefreshCw, Globe, Trash2, Loader2, ExternalLink } from "lucide-react";

interface AccountTableRowProps {
  index: number;
  account: models.GoogleAccountResponse;
  isActive: boolean;
  imageEnabled: boolean;
  videoEnabled: boolean;
  proxyValue?: string;
  credit?: number;
  onToggleActive: (id: string, active: boolean) => void;
  onToggleImage: (id: string, checked: boolean) => void;
  onToggleVideo: (id: string, checked: boolean) => void;
  onRefresh: (id: string) => Promise<void>;
  onOpenProxyModal: (account: models.GoogleAccountResponse) => void;
  onOpenBrowser: (id: string) => Promise<void>;
  onDelete: (id: string) => void;
}

export function AccountTableRow({
  index,
  account,
  isActive,
  imageEnabled,
  videoEnabled,
  proxyValue = "-",
  credit,
  onToggleActive,
  onToggleImage,
  onToggleVideo,
  onRefresh,
  onOpenProxyModal,
  onOpenBrowser,
  onDelete,
}: AccountTableRowProps) {
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isOpeningBrowser, setIsOpeningBrowser] = useState(false);

  const handleRefresh = async () => {
    setIsRefreshing(true);
    try {
      await onRefresh(account.id);
    } finally {
      setIsRefreshing(false);
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

  // Real credits from database
  const realCredits = account.credits !== undefined && account.credits !== null ? account.credits : (credit ?? 1050);
  const displayCredits = realCredits.toLocaleString();
  const displayTier = account.tier || "PRO";
  const hasProxy = proxyValue && proxyValue !== "-";

  return (
    <tr className="border-b border-border/50 hover:bg-muted/40 transition-colors duration-100">
      {/* 1. STT */}
      <td className="py-2 px-1 text-center text-xs font-medium text-muted-foreground select-none">
        {index}
      </td>

      {/* 2. TẮT/BẬT */}
      <td className="py-2 px-0.5 text-center">
        <div className="flex justify-center items-center">
          <ToggleSwitch
            size="sm"
            checked={isActive}
            onChange={(checked) => onToggleActive(account.id, checked)}
          />
        </div>
      </td>

      {/* 3. Ảnh Checkbox */}
      <td className="py-2 px-0.5 text-center">
        <div className="flex justify-center items-center">
          <button
            type="button"
            role="checkbox"
            aria-checked={imageEnabled}
            onClick={() => onToggleImage(account.id, !imageEnabled)}
            className={`flex size-4 items-center justify-center rounded cursor-pointer transition-colors border ${
              imageEnabled
                ? "bg-primary border-primary text-primary-foreground"
                : "border-input bg-background hover:bg-muted"
            }`}
            title={imageEnabled ? "Đang bật tạo ảnh (nhấp để tắt)" : "Đang tắt tạo ảnh (nhấp để bật)"}
          >
            {imageEnabled && <Check className="size-3 stroke-[3]" />}
          </button>
        </div>
      </td>

      {/* 4. Video Checkbox */}
      <td className="py-2 px-0.5 text-center">
        <div className="flex justify-center items-center">
          <button
            type="button"
            role="checkbox"
            aria-checked={videoEnabled}
            onClick={() => onToggleVideo(account.id, !videoEnabled)}
            className={`flex size-4 items-center justify-center rounded cursor-pointer transition-colors border ${
              videoEnabled
                ? "bg-primary border-primary text-primary-foreground"
                : "border-input bg-background hover:bg-muted"
            }`}
            title={videoEnabled ? "Đang bật tạo video (nhấp để tắt)" : "Đang tắt tạo video (nhấp để bật)"}
          >
            {videoEnabled && <Check className="size-3 stroke-[3]" />}
          </button>
        </div>
      </td>

      {/* 5. Tài khoản Email (Real info) */}
      <td className="py-2 px-2 min-w-0">
        <div className="flex items-center gap-2">
          <div className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary text-[10px] font-bold uppercase select-none">
            {account.email.charAt(0) || "G"}
          </div>
          <div className="flex flex-col min-w-0">
            <span
              className="text-xs font-medium text-foreground truncate select-all"
              title={account.email}
            >
              {account.email}
            </span>
            {account.lastRefreshAt && (
              <span className="text-[10px] text-muted-foreground truncate">
                Đồng bộ: {account.lastRefreshAt.slice(11, 16)}
              </span>
            )}
          </div>
        </div>
      </td>

      {/* 6. Loại Hạng (PRO) */}
      <td className="py-2 px-0.5 text-center">
        <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-bold tracking-wide uppercase bg-violet-500/10 text-violet-600 dark:text-violet-400 border border-violet-500/20 whitespace-nowrap">
          {displayTier}
        </span>
      </td>

      {/* 7. Tín dụng (Credits từ database) */}
      <td className="py-2 px-1 text-center text-xs font-semibold text-foreground font-mono whitespace-nowrap">
        {displayCredits}
      </td>

      {/* 8. Proxy */}
      <td className="py-2 px-0.5 text-center">
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
          <span className="truncate max-w-[45px]">{hasProxy ? proxyValue : "-"}</span>
        </button>
      </td>

      {/* 9. Trạng thái */}
      <td className="py-2 px-1 text-center">
        {statusBadge}
      </td>

      {/* 10. Hành động (4 nút gọn gàng) */}
      <td className="py-2 px-1 text-center">
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
