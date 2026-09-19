import React, { useState, useMemo } from "react";
import { models } from "@/../wailsjs/go/models";
import { AccountTableRow } from "./AccountTableRow";
import {
  FolderDown,
  RefreshCw,
  Plus,
  Sparkles,
  AlertCircle,
  Download,
  Users,
  Zap,
  Search,
  X,
  Trash2,
} from "lucide-react";

interface AccountTableProps {
  accounts: models.GoogleAccountResponse[];
  isLoading: boolean;
  onRefreshAll: () => Promise<void>;
  onSyncLiveMetrics: () => Promise<void>;
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
  liveMetricsByAccount: Record<string, models.LiveAccountMetrics>;
  isLiveRefreshing?: boolean;
  onPurgeCaches?: () => Promise<void>;
  isPurging?: boolean;
  refreshingAccountId?: string | null;
}

export function AccountTable({
  accounts,
  isLoading,
  onRefreshAll,
  onSyncLiveMetrics,
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
  liveMetricsByAccount,
  isLiveRefreshing = false,
  onPurgeCaches,
  isPurging = false,
  refreshingAccountId,
}: AccountTableProps) {
  // Filter & Search states
  const [statusFilter, setStatusFilter] = useState<string>("ALL");
  const [searchQuery, setSearchQuery] = useState("");
  const [isRefreshingAll, setIsRefreshingAll] = useState(false);

  const handleRefreshAll = async () => {
    setIsRefreshingAll(true);
    try {
      await onRefreshAll();
    } finally {
      setIsRefreshingAll(false);
    }
  };

  // Metrics aggregation
  const activeAccountsCount = accounts.filter((a) => a.status === "ACTIVE").length;
  const disabledAccountsCount = accounts.filter((a) => a.status === "DISABLED").length;
  const expiredAccountsCount = accounts.filter(
    (a) => a.status === "EXPIRED" || a.status === "ERROR"
  ).length;

  const totalLiveFlowCredits = accounts.reduce((sum, acc) => {
    const flow = liveMetricsByAccount[acc.id]?.flow;
    return sum + (flow?.hasTotalCredits ? flow.totalCredits : 0);
  }, 0);

  const liveGeminiCount = accounts.filter(
    (acc) => liveMetricsByAccount[acc.id]?.gemini.available
  ).length;

  // Filtered accounts based on status filter and search query
  const filteredAccounts = useMemo(() => {
    return accounts.filter((acc) => {
      // 1. Status Filter
      if (statusFilter === "ACTIVE" && acc.status !== "ACTIVE") return false;
      if (
        statusFilter === "EXPIRED" &&
        acc.status !== "EXPIRED" &&
        acc.status !== "ERROR"
      )
        return false;
      if (statusFilter === "DISABLED" && acc.status !== "DISABLED") return false;

      // 2. Search Query (Email, Proxy, Tier)
      if (searchQuery.trim()) {
        const query = searchQuery.trim().toLowerCase();
        const emailMatch = acc.email.toLowerCase().includes(query);
        const proxyMatch = (acc.proxy || "").toLowerCase().includes(query);
        const tierMatch = (acc.tier || "").toLowerCase().includes(query);
        if (!emailMatch && !proxyMatch && !tierMatch) return false;
      }

      return true;
    });
  }, [accounts, statusFilter, searchQuery]);

  return (
    <div className="w-full space-y-4 font-sans">
      {/* 1. Quick Stats Metric Cards - Chiều rộng ngắn lại, gọn gàng */}
      <div className="flex flex-wrap items-center gap-2.5">
        {/* Card 1: Hoạt động */}
        <div className="w-[165px] sm:w-[185px] rounded-xl border border-border/80 bg-card p-2.5 px-3 shadow-2xs transition-all hover:border-border hover:shadow-xs">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-muted-foreground">Tài khoản hoạt động</span>
            <div className="flex size-6 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
              <Users className="size-3" />
            </div>
          </div>
          <div className="mt-1 flex items-baseline gap-1.5">
            <span className="text-lg font-bold font-mono text-foreground tracking-tight">
              {activeAccountsCount}
            </span>
            <span className="text-xs text-muted-foreground font-mono">/ {accounts.length}</span>
          </div>
        </div>

        {/* Card 2: Quota Flow snapshot */}
        <div className="w-[165px] sm:w-[185px] rounded-xl border border-border/80 bg-card p-2.5 px-3 shadow-2xs transition-all hover:border-border hover:shadow-xs">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-muted-foreground">Hạn mức Flow</span>
            <div className="flex size-6 items-center justify-center rounded-lg bg-sky-500/10 text-sky-600 dark:text-sky-400">
              <Zap className="size-3" />
            </div>
          </div>
          <div className="mt-1 flex items-baseline gap-1.5">
            <span className="text-lg font-bold font-mono text-foreground tracking-tight">
              {totalLiveFlowCredits.toLocaleString()}
            </span>
            <span className="text-xs font-medium text-muted-foreground">credit</span>
          </div>
        </div>

        {/* Card 3: Gemini snapshot */}
        <div className="w-[165px] sm:w-[185px] rounded-xl border border-border/80 bg-card p-2.5 px-3 shadow-2xs transition-all hover:border-border hover:shadow-xs">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-muted-foreground">Gemini AI</span>
            <div className="flex size-6 items-center justify-center rounded-lg bg-violet-500/10 text-violet-600 dark:text-violet-400">
              <Sparkles className="size-3" />
            </div>
          </div>
          <div className="mt-1 flex items-baseline gap-1.5">
            <span className="text-lg font-bold font-mono text-foreground tracking-tight">
              {liveGeminiCount}
            </span>
            <span className="text-xs text-muted-foreground font-mono">/ {accounts.length} sẵn sàng</span>
          </div>
        </div>
      </div>

      {/* 2. Main Card: Table & Control Toolbar */}
      <div className="w-full rounded-2xl border border-border/80 bg-card text-card-foreground shadow-xs overflow-hidden">
        {/* Header Bar */}
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 px-4 sm:px-6 py-3.5 border-b border-border/80 bg-muted/15">
          <div className="flex items-center gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-primary/20 via-primary/10 to-transparent border border-primary/25 text-primary shadow-xs">
              <Sparkles className="size-4" />
            </div>
            <div>
              <h3 className="text-sm sm:text-base font-semibold text-foreground tracking-tight">
                Danh sách tài khoản Google
              </h3>
              <p className="text-xs text-muted-foreground">
                Quản lý phiên đăng nhập Google Flow, Gemini, hạn mức quota và cấu hình Proxy
              </p>
            </div>
          </div>

          {/* Top Action Buttons */}
          <div className="flex items-center gap-2 shrink-0">
            {/* Làm mới tất cả */}
            <button
              type="button"
              onClick={handleRefreshAll}
              disabled={isRefreshingAll || accounts.length === 0}
              className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border/80 bg-background hover:bg-muted font-medium text-foreground text-xs shadow-2xs transition-all active:scale-95 disabled:opacity-50 cursor-pointer whitespace-nowrap"
              title="Quét và làm mới toàn bộ Cookies/Token các tài khoản"
            >
              <RefreshCw
                className={`size-3.5 text-muted-foreground ${
                  isRefreshingAll ? "animate-spin text-primary" : ""
                }`}
              />
              <span>{isRefreshingAll ? "Đang làm mới..." : "Làm mới tất cả"}</span>
            </button>

            {/* Đồng bộ quota thủ công; không polling Google nền */}
            <button
              type="button"
              onClick={onSyncLiveMetrics}
              disabled={isLiveRefreshing || activeAccountsCount === 0}
              className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border/80 bg-background hover:bg-muted font-medium text-foreground text-xs shadow-2xs transition-all active:scale-95 disabled:opacity-50 cursor-pointer whitespace-nowrap"
              title="Đọc quota Flow/Gemini một lần theo cache an toàn"
            >
              <Zap className={`size-3.5 text-sky-500 ${isLiveRefreshing ? "animate-pulse" : ""}`} />
              <span>{isLiveRefreshing ? "Đang đọc quota..." : "Đồng bộ quota"}</span>
            </button>

            {/* Thêm danh sách (Bulk Add) */}
            <button
              type="button"
              onClick={onOpenBulkAdd}
              className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border/80 bg-background hover:bg-muted font-medium text-foreground text-xs shadow-2xs transition-all active:scale-95 cursor-pointer whitespace-nowrap"
              title="Thêm hàng loạt tài khoản từ email và Cookie"
            >
              <Users className="size-3.5 text-primary" />
              <span>Thêm danh sách</span>
            </button>

            {/* Thêm tài khoản (Primary CTA) */}
            <button
              type="button"
              onClick={onAddAccount}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-primary hover:bg-primary/90 text-primary-foreground font-semibold text-xs shadow-xs transition-all active:scale-95 cursor-pointer whitespace-nowrap"
              title="Đăng nhập qua Chrome thật hoặc nhập Cookie thủ công"
            >
              <Plus className="size-3.5 stroke-[2.5]" />
              <span>Thêm tài khoản</span>
            </button>
          </div>
        </div>

        {/* Toolbar Bar: 1 dòng duy nhất theo thứ tự: Tìm kiếm -> Trạng thái hoạt động -> 3 nút */}
        <div className="flex flex-wrap lg:flex-nowrap items-center justify-between gap-2 px-4 sm:px-6 py-2 border-b border-border/70 bg-background/50">
          {/* Nhóm trái: Tìm kiếm + Trạng thái hoạt động */}
          <div className="flex items-center gap-2 sm:gap-2.5 shrink-0 flex-wrap sm:flex-nowrap">
            {/* 1. Tìm kiếm */}
            <div className="relative w-40 sm:w-48 shrink-0">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Tìm email, proxy..."
                className="w-full rounded-lg border border-border/80 bg-background py-1 pl-8 pr-7 text-xs text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary transition-colors"
              />
              {searchQuery && (
                <button
                  type="button"
                  onClick={() => setSearchQuery("")}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground cursor-pointer"
                >
                  <X className="size-3.5" />
                </button>
              )}
            </div>

            {/* 2. Trạng thái hoạt động (Filter Tabs) */}
            <div className="flex items-center rounded-lg border border-border/80 bg-muted/30 p-0.5 text-xs font-medium shrink-0">
              {[
                { key: "ALL", label: "Tất cả", count: accounts.length },
                { key: "ACTIVE", label: "Hoạt động", count: activeAccountsCount },
                { key: "DISABLED", label: "Đã tắt", count: disabledAccountsCount },
                { key: "EXPIRED", label: "Hết hạn", count: expiredAccountsCount },
              ].map((item) => (
                <button
                  key={item.key}
                  type="button"
                  onClick={() => setStatusFilter(item.key)}
                  className={`flex items-center gap-1 px-2 py-0.5 rounded-md transition-all cursor-pointer select-none text-[11px] whitespace-nowrap ${
                    statusFilter === item.key
                      ? "bg-background text-foreground font-semibold shadow-2xs"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  <span>{item.label}</span>
                  <span
                    className={`rounded-full px-1.5 py-0.2 text-[10px] font-mono ${
                      statusFilter === item.key
                        ? "bg-primary/10 text-primary font-bold"
                        : "bg-muted text-muted-foreground"
                    }`}
                  >
                    {item.count}
                  </span>
                </button>
              ))}
            </div>
          </div>

          {/* 3. 3 nút: Nạp sao lưu, Sao lưu ZIP, Dọn rác Profile */}
          <div className="flex items-center gap-1.5 shrink-0">
            {/* Nút 1: Nạp sao lưu */}
            <button
              type="button"
              onClick={onImportBackup}
              disabled={isImporting}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border/80 bg-background hover:bg-muted font-medium text-foreground text-xs transition-all active:scale-95 disabled:opacity-50 cursor-pointer shadow-2xs whitespace-nowrap"
              title="Nạp tài khoản và hồ sơ Chrome từ file zip hoặc thư mục sao lưu G-Labs"
            >
              <FolderDown className="size-3.5 text-violet-500 shrink-0" />
              <span>{isImporting ? "Đang nạp..." : "Nạp sao lưu"}</span>
            </button>

            {/* Nút 2: Xuất sao lưu ZIP */}
            <button
              type="button"
              onClick={onExportBackup}
              disabled={isExporting || accounts.length === 0}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border/80 bg-background hover:bg-muted font-medium text-foreground text-xs transition-all active:scale-95 disabled:opacity-50 cursor-pointer shadow-2xs whitespace-nowrap"
              title="Đóng gói và xuất toàn bộ tài khoản cùng hồ sơ Chrome ra file ZIP"
            >
              <Download className="size-3.5 text-emerald-500 shrink-0" />
              <span>{isExporting ? "Đang xuất..." : "Sao lưu ZIP"}</span>
            </button>

            {/* Nút 3: Dọn rác Profile */}
            {onPurgeCaches && (
              <button
                type="button"
                onClick={onPurgeCaches}
                disabled={isPurging}
                className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border/80 bg-background hover:bg-muted font-medium text-foreground text-xs transition-all active:scale-95 disabled:opacity-50 cursor-pointer shadow-2xs whitespace-nowrap"
                title="Xóa cache trình duyệt (Code Cache, GPUCache) để giải phóng dung lượng ổ cứng"
              >
                <Trash2
                  className={`size-3.5 text-amber-500 shrink-0 ${
                    isPurging ? "animate-spin" : ""
                  }`}
                />
                <span>{isPurging ? "Đang dọn..." : "Dọn rác Profile"}</span>
              </button>
            )}
          </div>
        </div>

        {/* 3. Table View - 100% Fit, Zero Horizontal Scroll */}
        <div className="w-full overflow-hidden">
          <table className="w-full table-fixed border-collapse">
            <colgroup>
              <col style={{ width: "4%" }} />   {/* 1. # */}
              <col style={{ width: "7.5%" }} /> {/* 2. Bật/Tắt */}
              <col style={{ width: "23.5%" }} />{/* 3. Tài khoản Google */}
              <col style={{ width: "7%" }} />   {/* 4. Gói */}
              <col style={{ width: "22%" }} />  {/* 5. Quota realtime */}
              <col style={{ width: "11%" }} />  {/* 6. Proxy */}
              <col style={{ width: "11%" }} />  {/* 7. Trạng thái */}
              <col style={{ width: "14%" }} />  {/* 8. Thao tác */}
            </colgroup>
            <thead>
              <tr className="border-b border-border/80 bg-muted/30 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider select-none">
                <th className="py-3 px-1 text-center">#</th>
                <th className="py-3 px-1 text-center">Bật/Tắt</th>
                <th className="py-3 px-2 text-center">Tài khoản Google</th>
                <th className="py-3 px-1 text-center">Gói</th>
                <th className="py-3 px-2 text-center">Quota đã đồng bộ</th>
                <th className="py-3 px-1 text-center">Proxy</th>
                <th className="py-3 px-1 text-center">Trạng thái</th>
                <th className="py-3 px-1 text-center">Thao tác</th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr>
                  <td colSpan={8} className="py-16 text-center text-muted-foreground">
                    <div className="flex flex-col items-center justify-center gap-3">
                      <RefreshCw className="size-7 animate-spin text-primary" />
                      <span className="text-sm font-medium">Đang tải danh sách tài khoản...</span>
                    </div>
                  </td>
                </tr>
              ) : filteredAccounts.length === 0 ? (
                <tr>
                  <td colSpan={8} className="py-16 text-center">
                    {accounts.length === 0 ? (
                      /* Zero accounts empty state */
                      <div className="flex flex-col items-center justify-center gap-3 max-w-md mx-auto text-center px-4">
                        <div className="flex size-14 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                          <Users className="size-7" />
                        </div>
                        <h4 className="text-base font-semibold text-foreground">
                          Chưa có tài khoản Google nào
                        </h4>
                        <p className="text-xs text-muted-foreground leading-relaxed">
                          Đăng nhập qua Google Chrome hoặc nhập Cookie phiên làm việc để bắt đầu sử dụng Google Flow (Veo, Imagen) và Gemini AI với cơ chế xoay vòng thông minh.
                        </p>
                        <div className="flex flex-wrap items-center justify-center gap-2 mt-2">
                          <button
                            type="button"
                            onClick={onAddAccount}
                            className="flex items-center gap-1.5 px-3.5 py-2 rounded-lg bg-primary hover:bg-primary/90 text-primary-foreground font-semibold text-xs shadow-xs transition-all cursor-pointer"
                          >
                            <Plus className="size-3.5" />
                            <span>Thêm tài khoản đầu tiên</span>
                          </button>
                          <button
                            type="button"
                            onClick={onOpenBulkAdd}
                            className="flex items-center gap-1.5 px-3.5 py-2 rounded-lg border border-border bg-background hover:bg-muted text-foreground font-medium text-xs shadow-2xs transition-all cursor-pointer"
                          >
                            <Users className="size-3.5 text-primary" />
                            <span>Thêm danh sách</span>
                          </button>
                          <button
                            type="button"
                            onClick={onImportBackup}
                            className="flex items-center gap-1.5 px-3.5 py-2 rounded-lg border border-border bg-background hover:bg-muted text-foreground font-medium text-xs shadow-2xs transition-all cursor-pointer"
                          >
                            <FolderDown className="size-3.5 text-violet-500" />
                            <span>Nạp sao lưu</span>
                          </button>
                        </div>
                      </div>
                    ) : (
                      /* Filter empty state */
                      <div className="flex flex-col items-center justify-center gap-2 py-6 text-muted-foreground">
                        <AlertCircle className="size-8 text-muted-foreground/40" />
                        <p className="text-sm font-medium text-foreground">
                          Không tìm thấy tài khoản phù hợp
                        </p>
                        <p className="text-xs">
                          Không có tài khoản nào khớp với bộ lọc hoặc từ khóa "{searchQuery}".
                        </p>
                        <button
                          type="button"
                          onClick={() => {
                            setSearchQuery("");
                            setStatusFilter("ALL");
                          }}
                          className="mt-2 text-xs font-semibold text-primary hover:underline cursor-pointer"
                        >
                          Đặt lại bộ lọc
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              ) : (
                filteredAccounts.map((acc, i) => {
                  const isActive = acc.status === "ACTIVE";
                  const proxy = acc.proxy || "-";

                  return (
                    <AccountTableRow
                      key={acc.id}
                      index={i + 1}
                      account={acc}
                      isActive={isActive}
                      proxyValue={proxy}
                      liveMetrics={liveMetricsByAccount[acc.id]}
                      onToggleActive={onToggleActive}
                      onRefresh={onRefreshAccount}
                      onTest={onTestAccount}
                      onOpenProxyModal={onOpenProxyModal}
                      onOpenBrowser={onOpenBrowser}
                      onDelete={onDeleteAccount}
                      isRefreshing={acc.id === refreshingAccountId}
                    />
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        {/* 4. Table Footer Bar */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 px-6 py-3 border-t border-border/70 bg-muted/10 text-[11px] text-muted-foreground select-none">
          <div>
            Hiển thị <span className="font-semibold text-foreground">{filteredAccounts.length}</span> trên tổng số{" "}
            <span className="font-semibold text-foreground">{accounts.length}</span> tài khoản
          </div>
          <div className="flex items-center gap-3">
            <span>Bấm vào email để sao chép</span>
            <span>·</span>
            <span>Bấm Proxy để cấu hình</span>
            <span>·</span>
            <span className="flex items-center gap-1">
              <span
                className={`size-1.5 rounded-full ${
                  isLiveRefreshing ? "bg-amber-500 animate-pulse" : "bg-emerald-500"
                }`}
              />
              Quota cập nhật khi cần và đồng bộ nền mỗi 5 phút
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
