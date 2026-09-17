import React, { useState } from "react";
import { models } from "@/../wailsjs/go/models";
import { AccountTableRow } from "./AccountTableRow";
import { ToggleSwitch } from "./ToggleSwitch";
import {
  FolderDown,
  RefreshCw,
  Plus,
  Filter,
  Check,
  Sparkles,
  ShieldCheck,
  AlertCircle,
} from "lucide-react";

interface AccountTableProps {
  accounts: models.GoogleAccountResponse[];
  isLoading: boolean;
  onRefreshAll: () => Promise<void>;
  onAddAccount: () => void;
  onImportBackup: () => Promise<void>;
  isImporting?: boolean;
  onToggleActive: (id: string, active: boolean) => void;
  onToggleImage: (id: string, checked: boolean) => void;
  onToggleVideo: (id: string, checked: boolean) => void;
  onBulkToggleFeature: (feature: "image" | "video", checked: boolean) => void;
  onRefreshAccount: (id: string) => Promise<void>;
  onOpenBrowser: (id: string) => Promise<void>;
  onDeleteAccount: (id: string) => void;
  onOpenProxyModal: (account: models.GoogleAccountResponse) => void;
  proxies: Record<string, string>;
}

export function AccountTable({
  accounts,
  isLoading,
  onRefreshAll,
  onAddAccount,
  onImportBackup,
  isImporting = false,
  onToggleActive,
  onToggleImage,
  onToggleVideo,
  onBulkToggleFeature,
  onRefreshAccount,
  onOpenBrowser,
  onDeleteAccount,
  onOpenProxyModal,
  proxies,
}: AccountTableProps) {
  // Real bulk toggle states derived from accounts in database
  const allImageEnabled =
    accounts.length > 0 && accounts.every((a) => a.imageEnabled !== false);
  const allVideoEnabled =
    accounts.length > 0 && accounts.every((a) => a.videoEnabled !== false);

  const [autoDisableExhausted, setAutoDisableExhausted] = useState(() => {
    return localStorage.getItem("dezuxk_auto_disable_exhausted") === "true";
  });

  // Status Filter ("ALL", "ACTIVE", "EXPIRED", "DISABLED")
  const [statusFilter, setStatusFilter] = useState<string>("ALL");
  const [isFilterMenuOpen, setIsFilterMenuOpen] = useState(false);

  // Bulk refresh loading state
  const [isRefreshingAll, setIsRefreshingAll] = useState(false);

  const handleToggleAutoDisable = (checked: boolean) => {
    setAutoDisableExhausted(checked);
    localStorage.setItem("dezuxk_auto_disable_exhausted", String(checked));
  };

  const handleRefreshAll = async () => {
    setIsRefreshingAll(true);
    try {
      await onRefreshAll();
    } finally {
      setIsRefreshingAll(false);
    }
  };

  // Filter accounts
  const filteredAccounts = accounts.filter((acc) => {
    if (statusFilter === "ACTIVE") return acc.status === "ACTIVE";
    if (statusFilter === "EXPIRED") return acc.status === "EXPIRED" || acc.status === "ERROR";
    if (statusFilter === "DISABLED") return acc.status === "DISABLED";
    return true;
  });

  const activeAccountsCount = accounts.filter((a) => a.status === "ACTIVE").length;

  // Calculate sum of credits directly from database
  const totalCredits = accounts.reduce((sum, acc) => {
    if (acc.status === "ACTIVE" || acc.status === "") {
      return sum + (acc.credits ?? 1050);
    }
    return sum;
  }, 0);

  return (
    <div className="w-full rounded-xl border border-border bg-card text-card-foreground shadow-xs overflow-hidden font-sans">
      {/* 1. Header Information Bar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-6 py-4 border-b border-border bg-muted/20">
        <div className="flex items-center gap-2.5">
          <div className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <Sparkles className="size-5" />
          </div>
          <div>
            <h3 className="text-base font-semibold text-foreground tracking-tight">
              Tài khoản Flow & Google
            </h3>
            <p className="text-xs text-muted-foreground">
              Quản lý phiên đăng nhập, cấp phát credit và cấu hình Proxy cho từng tài khoản
            </p>
          </div>
        </div>

        {/* Live Counters */}
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2 rounded-lg border border-border bg-background px-3 py-1.5 text-xs">
            <span className="text-muted-foreground">Đang hoạt động:</span>
            <span className="font-bold text-emerald-600 dark:text-emerald-400 font-mono">
              {activeAccountsCount}
            </span>
            <span className="text-muted-foreground">/ {accounts.length}</span>
          </div>

          <div className="flex items-center gap-2 rounded-lg border border-border bg-background px-3 py-1.5 text-xs">
            <span className="text-muted-foreground">Tổng tín dụng:</span>
            <span className="font-bold text-primary font-mono">
              {totalCredits.toLocaleString()} credit
            </span>
          </div>
        </div>
      </div>

      {/* 2. Operations & Filter Bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-3.5 border-b border-border/80 bg-background/50 text-xs">
        {/* Left: Bulk toggles */}
        <div className="flex flex-wrap items-center gap-5">
          {/* Quick select: Checkbox Ảnh & Video */}
          <div className="flex items-center gap-2">
            <span className="font-medium text-muted-foreground">Bật tất cả:</span>
            <label className="flex items-center gap-1.5 cursor-pointer select-none text-foreground font-medium">
              <button
                type="button"
                role="checkbox"
                aria-checked={allImageEnabled}
                onClick={() => onBulkToggleFeature("image", !allImageEnabled)}
                className={`flex size-4 items-center justify-center rounded border transition-colors ${
                  allImageEnabled
                    ? "bg-primary border-primary text-primary-foreground"
                    : "border-input bg-background"
                }`}
              >
                {allImageEnabled && <Check className="size-3 stroke-[3]" />}
              </button>
              Ảnh
            </label>

            <label className="flex items-center gap-1.5 cursor-pointer select-none text-foreground font-medium ml-1">
              <button
                type="button"
                role="checkbox"
                aria-checked={allVideoEnabled}
                onClick={() => onBulkToggleFeature("video", !allVideoEnabled)}
                className={`flex size-4 items-center justify-center rounded border transition-colors ${
                  allVideoEnabled
                    ? "bg-primary border-primary text-primary-foreground"
                    : "border-input bg-background"
                }`}
              >
                {allVideoEnabled && <Check className="size-3 stroke-[3]" />}
              </button>
              Video
            </label>
          </div>

          {/* Auto Disable when limit reached */}
          <div className="flex items-center gap-2 pl-3 border-l border-border">
            <ToggleSwitch
              size="sm"
              checked={autoDisableExhausted}
              onChange={handleToggleAutoDisable}
            />
            <span className="text-muted-foreground select-none font-medium">
              Tự động tắt tài khoản hết hạn mức
            </span>
          </div>
        </div>

        {/* Right: Actions */}
        <div className="flex items-center gap-2.5">
          {/* Nút Nhập Backup Flow từ G-Labs */}
          <button
            type="button"
            onClick={onImportBackup}
            disabled={isImporting}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-violet-600 hover:bg-violet-700 text-white font-medium shadow-xs transition-all active:scale-95 disabled:opacity-50 cursor-pointer"
            title="Nhập tài khoản và profiles từ bản backup G-Labs (glabs-flow-accounts-20260917-160519)"
          >
            <FolderDown className="size-3.5" />
            <span>{isImporting ? "Đang nạp..." : "Nhập Backup Flow"}</span>
          </button>

          {/* Nút Thêm tài khoản qua Chrome thật */}
          <button
            type="button"
            onClick={onAddAccount}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-primary hover:bg-primary/90 text-primary-foreground font-medium shadow-xs transition-all active:scale-95 cursor-pointer"
            title="Mở trình duyệt Chrome thật độc lập để đăng nhập tài khoản Google"
          >
            <Plus className="size-3.5 stroke-[2.5]" />
            <span>Thêm tài khoản</span>
          </button>

          {/* Nút Làm mới tất cả */}
          <button
            type="button"
            onClick={handleRefreshAll}
            disabled={isRefreshingAll || accounts.length === 0}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-border bg-background hover:bg-muted font-medium text-foreground transition-all active:scale-95 disabled:opacity-50 cursor-pointer"
            title="Quét và làm mới toàn bộ Cookies/Token các tài khoản"
          >
            <RefreshCw
              className={`size-3.5 text-muted-foreground ${
                isRefreshingAll ? "animate-spin text-primary" : ""
              }`}
            />
            <span>Làm mới tất cả</span>
          </button>
        </div>
      </div>

      {/* 3. Table View - 100% Fit, Zero Horizontal Scroll */}
      <div className="w-full overflow-hidden">
        <table className="w-full table-fixed border-collapse">
          <colgroup>
            <col style={{ width: "32px" }} />  {/* 1. # */}
            <col style={{ width: "48px" }} />  {/* 2. Bật/Tắt */}
            <col style={{ width: "36px" }} />  {/* 3. Ảnh */}
            <col style={{ width: "36px" }} />  {/* 4. Video */}
            <col />                            {/* 5. Tài khoản (Auto fill remaining space) */}
            <col style={{ width: "48px" }} />  {/* 6. Loại */}
            <col style={{ width: "70px" }} />  {/* 7. Tín dụng */}
            <col style={{ width: "60px" }} />  {/* 8. Proxy */}
            <col style={{ width: "96px" }} />  {/* 9. Trạng thái */}
            <col style={{ width: "114px" }} /> {/* 10. Hành động */}
          </colgroup>
          <thead>
            <tr className="border-b border-border bg-muted/40 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider select-none">
              <th className="py-2.5 px-1 text-center">#</th>
              <th className="py-2.5 px-0.5 text-center">Bật/Tắt</th>
              <th className="py-2.5 px-0.5 text-center">Ảnh</th>
              <th className="py-2.5 px-0.5 text-center">Video</th>
              <th className="py-2.5 px-2 text-left">Tài khoản</th>
              <th className="py-2.5 px-0.5 text-center">Loại</th>
              <th className="py-2.5 px-1 text-center">Tín dụng</th>
              <th className="py-2.5 px-0.5 text-center">Proxy</th>
              <th className="py-2.5 px-1 text-center relative">
                <div className="flex items-center justify-center gap-1">
                  <span>Trạng thái</span>
                  <button
                    type="button"
                    onClick={() => setIsFilterMenuOpen(!isFilterMenuOpen)}
                    className="p-0.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
                    title="Lọc trạng thái"
                  >
                    <Filter className="size-3" />
                  </button>
                </div>

                {/* Dropdown Filter Menu */}
                {isFilterMenuOpen && (
                  <>
                    <div
                      className="fixed inset-0 z-20"
                      onClick={() => setIsFilterMenuOpen(false)}
                    />
                    <div className="absolute right-0 top-full mt-1 w-32 rounded-lg border border-border bg-popover text-popover-foreground shadow-lg z-30 py-1 text-xs normal-case tracking-normal">
                      {[
                        { key: "ALL", label: "Tất cả" },
                        { key: "ACTIVE", label: "Hoạt động" },
                        { key: "DISABLED", label: "Đã tắt" },
                        { key: "EXPIRED", label: "Hết hạn" },
                      ].map((item) => (
                        <button
                          key={item.key}
                          type="button"
                          onClick={() => {
                            setStatusFilter(item.key);
                            setIsFilterMenuOpen(false);
                          }}
                          className={`w-full flex items-center justify-between px-3 py-1.5 text-left hover:bg-muted transition-colors ${
                            statusFilter === item.key
                              ? "font-semibold text-primary"
                              : "text-foreground"
                          }`}
                        >
                          <span>{item.label}</span>
                          {statusFilter === item.key && (
                            <Check className="size-3" />
                          )}
                        </button>
                      ))}
                    </div>
                  </>
                )}
              </th>
              <th className="py-2.5 px-1 text-center">Hành động</th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr>
                <td colSpan={10} className="py-12 text-center text-muted-foreground">
                  <div className="flex flex-col items-center justify-center gap-2">
                    <RefreshCw className="size-6 animate-spin text-primary" />
                    <span className="text-sm">Đang tải danh sách tài khoản...</span>
                  </div>
                </td>
              </tr>
            ) : filteredAccounts.length === 0 ? (
              <tr>
                <td colSpan={10} className="py-12 text-center">
                  <div className="flex flex-col items-center justify-center gap-2 text-muted-foreground">
                    <AlertCircle className="size-8 text-muted-foreground/50" />
                    <p className="text-sm font-medium text-foreground">
                      Chưa có tài khoản nào
                    </p>
                    <p className="text-xs max-w-sm">
                      Nhấn <b>"Nhập Backup Flow"</b> để nạp nhanh danh sách tài khoản từ G-Labs, hoặc nhấn <b>"Thêm tài khoản"</b> để đăng nhập Chrome thật.
                    </p>
                  </div>
                </td>
              </tr>
            ) : (
              filteredAccounts.map((acc, i) => {
                const isActive = acc.status === "ACTIVE";
                const imgEn = acc.imageEnabled ?? true;
                const vidEn = acc.videoEnabled ?? true;
                const proxy = acc.proxy || proxies[acc.id] || "-";

                return (
                  <AccountTableRow
                    key={acc.id}
                    index={i + 1}
                    account={acc}
                    isActive={isActive}
                    imageEnabled={imgEn}
                    videoEnabled={vidEn}
                    proxyValue={proxy}
                    credit={acc.credits ?? 1050}
                    onToggleActive={onToggleActive}
                    onToggleImage={onToggleImage}
                    onToggleVideo={onToggleVideo}
                    onRefresh={onRefreshAccount}
                    onOpenProxyModal={onOpenProxyModal}
                    onOpenBrowser={onOpenBrowser}
                    onDelete={onDeleteAccount}
                  />
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
