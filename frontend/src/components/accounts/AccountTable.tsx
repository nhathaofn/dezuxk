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
  Download,
  Users,
  Zap,
} from "lucide-react";

interface AccountTableProps {
  accounts: models.GoogleAccountResponse[];
  isLoading: boolean;
  onRefreshAll: () => Promise<void>;
  onAddAccount: () => void;
  onOpenBulkAdd: () => void;
  onExportBackup: () => Promise<void>;
  onImportBackup: () => Promise<void>;
  isExporting?: boolean;
  isImporting?: boolean;
  onToggleActive: (id: string, active: boolean) => void;
  onRefreshAccount: (id: string) => Promise<void>;
  onTestAccount: (id: string) => Promise<void>;
  onOpenBrowser: (id: string) => Promise<void>;
  onDeleteAccount: (id: string) => void;
  onOpenProxyModal: (account: models.GoogleAccountResponse) => void;
  onUpdateCredits?: (id: string, credits: number) => Promise<void>;
  proxies: Record<string, string>;
  refreshingAccountId?: string | null;
}

export function AccountTable({
  accounts,
  isLoading,
  onRefreshAll,
  onAddAccount,
  onOpenBulkAdd,
  onExportBackup,
  onImportBackup,
  isExporting = false,
  isImporting = false,
  onToggleActive,
  onRefreshAccount,
  onTestAccount,
  onOpenBrowser,
  onDeleteAccount,
  onOpenProxyModal,
  onUpdateCredits,
  proxies,
  refreshingAccountId,
}: AccountTableProps) {
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

  // Calculate sum of credits directly from database (100% dynamic)
  const totalCredits = accounts.reduce((sum, acc) => {
    if (acc.status === "ACTIVE" || acc.status === "") {
      return sum + (typeof acc.credits === "number" ? acc.credits : 0);
    }
    return sum;
  }, 0);

  return (
    <div className="w-full rounded-xl border border-border bg-card text-card-foreground shadow-xs overflow-hidden font-sans">
      {/* 1. Header Information Bar */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 px-6 py-4 border-b border-border bg-muted/20">
        <div className="flex items-center gap-3 min-w-0">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-primary/20 via-primary/10 to-transparent border border-primary/25 text-primary shadow-xs">
            <Sparkles className="size-5" />
          </div>
          <div className="min-w-0">
            <h3 className="text-base font-semibold text-foreground tracking-tight whitespace-nowrap">
              Tài khoản Google & Flow
            </h3>
            <p className="text-xs text-muted-foreground truncate">
              Quản lý phiên đăng nhập, hạn mức credit và cấu hình Proxy
            </p>
          </div>
        </div>

        {/* Live Counters */}
        <div className="flex items-center gap-2.5 shrink-0">
          {/* Card 1: Hoạt động */}
          <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg border border-emerald-500/25 bg-emerald-500/10 dark:bg-emerald-500/15 text-xs whitespace-nowrap shadow-2xs">
            <span className="relative flex size-2 shrink-0">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75"></span>
              <span className="relative inline-flex size-2 rounded-full bg-emerald-500"></span>
            </span>
            <span className="text-muted-foreground font-medium">Hoạt động:</span>
            <span className="font-bold text-emerald-600 dark:text-emerald-400 font-mono text-sm">
              {activeAccountsCount}
            </span>
            <span className="text-muted-foreground/60 text-xs font-mono">/ {accounts.length}</span>
          </div>

          {/* Card 2: Tổng tín dụng */}
          <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg border border-primary/25 bg-primary/10 dark:bg-primary/15 text-xs whitespace-nowrap shadow-2xs">
            <Zap className="size-3.5 text-primary shrink-0" />
            <span className="text-muted-foreground font-medium">Tổng tín dụng:</span>
            <span className="font-bold text-foreground font-mono text-sm">
              {totalCredits.toLocaleString()}
            </span>
            <span className="text-[11px] text-muted-foreground font-medium">credit</span>
          </div>
        </div>
      </div>

      {/* 2. Operations & Filter Bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-3.5 border-b border-border/80 bg-background/50 text-xs">
        {/* Left: Auto Disable when limit reached */}
        <div className="flex items-center gap-2">
          <ToggleSwitch
            size="sm"
            checked={autoDisableExhausted}
            onChange={handleToggleAutoDisable}
          />
          <span className="text-muted-foreground select-none font-medium">
            Tự động tắt tài khoản hết hạn mức
          </span>
        </div>

        {/* Right: Actions */}
        <div className="flex flex-wrap items-center gap-2">
          {/* Nút Nạp sao lưu */}
          <button
            type="button"
            onClick={onImportBackup}
            disabled={isImporting}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-violet-600 hover:bg-violet-700 text-white font-medium shadow-xs transition-all active:scale-95 disabled:opacity-50 cursor-pointer text-xs"
            title="Nạp tài khoản và hồ sơ Chrome từ file zip hoặc thư mục sao lưu G-Labs"
          >
            <FolderDown className="size-3.5" />
            <span>{isImporting ? "Đang nạp..." : "Nạp sao lưu"}</span>
          </button>

          {/* Nút Xuất sao lưu ZIP */}
          <button
            type="button"
            onClick={onExportBackup}
            disabled={isExporting || accounts.length === 0}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-700 text-white font-medium shadow-xs transition-all active:scale-95 disabled:opacity-50 cursor-pointer text-xs"
            title="Đóng gói và xuất toàn bộ tài khoản cùng hồ sơ Chrome ra file ZIP"
          >
            <Download className="size-3.5" />
            <span>{isExporting ? "Đang xuất..." : "Sao lưu ZIP"}</span>
          </button>

          {/* Nút Thêm theo danh sách (Bulk Add) */}
          <button
            type="button"
            onClick={onOpenBulkAdd}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white font-medium shadow-xs transition-all active:scale-95 cursor-pointer text-xs"
            title="Thêm hàng loạt tài khoản từ danh sách email|pass hoặc cookies"
          >
            <Users className="size-3.5" />
            <span>Thêm danh sách</span>
          </button>

          {/* Nút Thêm tài khoản đơn lẻ */}
          <button
            type="button"
            onClick={onAddAccount}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-primary hover:bg-primary/90 text-primary-foreground font-medium shadow-xs transition-all active:scale-95 cursor-pointer text-xs"
            title="Thêm tài khoản qua Chrome thật hoặc nhập Cookie thủ công"
          >
            <Plus className="size-3.5 stroke-[2.5]" />
            <span>Thêm tài khoản</span>
          </button>

          {/* Nút Làm mới tất cả */}
          <button
            type="button"
            onClick={handleRefreshAll}
            disabled={isRefreshingAll || accounts.length === 0}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border bg-background hover:bg-muted font-medium text-foreground transition-all active:scale-95 disabled:opacity-50 cursor-pointer text-xs"
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
            <col style={{ width: "36px" }} />  {/* 1. # */}
            <col style={{ width: "56px" }} />  {/* 2. Bật/Tắt */}
            <col style={{ width: "210px" }} /> {/* 3. Tài khoản */}
            <col style={{ width: "52px" }} />  {/* 4. Loại */}
            <col style={{ width: "76px" }} />  {/* 5. Tín dụng */}
            <col style={{ width: "84px" }} />  {/* 6. Proxy */}
            <col style={{ width: "105px" }} /> {/* 7. Trạng thái */}
            <col style={{ width: "135px" }} /> {/* 8. Hành động */}
          </colgroup>
          <thead>
            <tr className="border-b border-border bg-muted/40 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider select-none">
              <th className="py-2.5 px-1 text-center">#</th>
              <th className="py-2.5 px-1 text-center">Bật/Tắt</th>
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
                <td colSpan={8} className="py-12 text-center text-muted-foreground">
                  <div className="flex flex-col items-center justify-center gap-2">
                    <RefreshCw className="size-6 animate-spin text-primary" />
                    <span className="text-sm">Đang tải danh sách tài khoản...</span>
                  </div>
                </td>
              </tr>
            ) : filteredAccounts.length === 0 ? (
              <tr>
                <td colSpan={8} className="py-12 text-center">
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
                const proxy = acc.proxy || proxies[acc.id] || "-";

                return (
                  <AccountTableRow
                    key={acc.id}
                    index={i + 1}
                    account={acc}
                    isActive={isActive}
                    proxyValue={proxy}
                    credit={typeof acc.credits === "number" ? acc.credits : 0}
                    onToggleActive={onToggleActive}
                    onRefresh={onRefreshAccount}
                    onTest={onTestAccount}
                    onOpenProxyModal={onOpenProxyModal}
                    onOpenBrowser={onOpenBrowser}
                    onDelete={onDeleteAccount}
                    onUpdateCredits={onUpdateCredits}
                    isRefreshing={acc.id === refreshingAccountId}
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
