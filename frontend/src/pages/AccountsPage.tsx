import React, { useEffect, useState, useRef } from "react";
import { models } from "@/../wailsjs/go/models";
import {
  ListGoogleAccounts,
  GetLiveGoogleAccountMetrics,
  StartGoogleLogin,
  GetGoogleLoginStatus,
  CancelGoogleLogin,
  DeleteGoogleAccount,
  RefreshGoogleAccount,
  RefreshAllGoogleAccounts,
  ToggleGoogleAccount,
  OpenGoogleAccountBrowser,
  GetGoogleAccountProxy,
  ImportAccountsBackupDialog,
  ExportAccountsBackupDialog,
  SaveGoogleAccountProxy,
  TestGoogleAccount,
  PurgeGoogleAccountCaches,
} from "@/../wailsjs/go/main/App";
import { ActionConfirmDialog } from "@/components/common/ActionConfirmDialog";
import { LoginProgressModal } from "@/components/accounts/LoginProgressModal";
import { AccountTable } from "@/components/accounts/AccountTable";
import { ProxyModal } from "@/components/accounts/ProxyModal";
import { AddAccountModal } from "@/components/accounts/AddAccountModal";
import { BulkAddModal } from "@/components/accounts/BulkAddModal";
import { toast } from "@/lib/toast";
import { MIN_PASSWORD_LENGTH } from "@/lib/constants";

export function AccountsPage() {
  const [accounts, setAccounts] = useState<models.GoogleAccountResponse[]>([]);
  const [liveMetricsByAccount, setLiveMetricsByAccount] = useState<Record<string, models.LiveAccountMetrics>>({});
  const [isLiveRefreshing, setIsLiveRefreshing] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isImporting, setIsImporting] = useState(false);
  const [isExporting, setIsExporting] = useState(false);
  const [isPurging, setIsPurging] = useState(false);

  // Modals state
  const [isAddAccountModalOpen, setIsAddAccountModalOpen] = useState(false);
  const [isBulkAddModalOpen, setIsBulkAddModalOpen] = useState(false);

  // Add Account via Real Chrome Login state
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [loginStep, setLoginStep] = useState<string>("INITIALIZING");
  const [loginMessage, setLoginMessage] = useState<string>("");
  const [loginError, setLoginError] = useState<string | undefined>();
  const [isLoginModalOpen, setIsLoginModalOpen] = useState(false);
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const accountsRef = useRef<models.GoogleAccountResponse[]>([]);
  const liveRefreshInFlightRef = useRef<Promise<void> | null>(null);
  const queuedLiveAccountIdsRef = useRef<string[]>([]);
  const liveRefreshActiveAccountIdsRef = useRef<Set<string>>(new Set());
  const liveRefreshStoppedRef = useRef(false);

  // Delete confirmation modal state
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  // Proxy modal state
  const [proxyTargetAccount, setProxyTargetAccount] = useState<models.GoogleAccountResponse | null>(null);
  const [proxyEditorValue, setProxyEditorValue] = useState("");

  const fetchAccounts = async () => {
    try {
      const data = await ListGoogleAccounts();
      const nextAccounts = data || [];
      accountsRef.current = nextAccounts;
      setAccounts(nextAccounts);
    } catch (err: any) {
      toast.error("Không thể tải danh sách tài khoản", err?.message || String(err));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchAccounts();
    return () => {
      if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
      liveRefreshStoppedRef.current = true;
    };
  }, []);

  useEffect(() => {
    accountsRef.current = accounts;
  }, [accounts]);

  // Read quota from the authenticated profile without touching SQLite. Calls
  // are serialized so multiple accounts do not launch competing Chrome jobs.
  // A second trigger is coalesced instead of launching another Chrome process.
  const refreshLiveMetrics = async (accountIds: string[]): Promise<void> => {
    const uniqueIds = Array.from(new Set(accountIds)).filter(Boolean);
    if (uniqueIds.length === 0) return;

    if (liveRefreshInFlightRef.current) {
      const activeIds = liveRefreshActiveAccountIdsRef.current;
      queuedLiveAccountIdsRef.current = Array.from(
        new Set([...queuedLiveAccountIdsRef.current, ...uniqueIds].filter((id) => !activeIds.has(id)))
      );
      return liveRefreshInFlightRef.current;
    }

    liveRefreshActiveAccountIdsRef.current = new Set(uniqueIds);
    setIsLiveRefreshing(true);
    const refreshPromise = (async () => {
      try {
        for (const accountId of uniqueIds) {
          try {
            const metrics = await GetLiveGoogleAccountMetrics(accountId);
            if (metrics) {
              setLiveMetricsByAccount((previous) => ({ ...previous, [accountId]: metrics }));
            }
          } catch {
            // Keep the last successful in-memory snapshot; a transient Chrome
            // lock or network failure must not erase useful live data.
          }
        }
      } finally {
        setIsLiveRefreshing(false);
        liveRefreshActiveAccountIdsRef.current.clear();
      }
    })();
    liveRefreshInFlightRef.current = refreshPromise;

    try {
      await refreshPromise;
    } finally {
      liveRefreshInFlightRef.current = null;
      const queuedIds = queuedLiveAccountIdsRef.current;
      queuedLiveAccountIdsRef.current = [];
      if (!liveRefreshStoppedRef.current && queuedIds.length > 0) {
        void refreshLiveMetrics(queuedIds);
      }
    }
  };

  // Quota sync is explicit while the gateway data plane is not yet producing
  // usage events. This keeps normal account-page navigation Google-free.
  const handleSyncLiveMetrics = async () => {
    const activeIds = accountsRef.current
      .filter((account) => account.status === "ACTIVE")
      .map((account) => account.id);
    if (activeIds.length === 0) {
      toast.info("Không có tài khoản", "Không có tài khoản hoạt động để đồng bộ quota.");
      return;
    }

    await refreshLiveMetrics(activeIds);
    toast.info("Đã xử lý đồng bộ quota", "Hệ thống dùng cache an toàn và không tự lặp request Google.");
  };

  // Handler: Start Interactive Real Chrome Login
  const handleStartAddAccount = async (proxy: string = "") => {
    try {
      setIsLoginModalOpen(true);
      setLoginStep("INITIALIZING");
      setLoginMessage("Đang quét tìm Google Chrome thật trên máy tính của bạn...");
      setLoginError(undefined);

      const res = await StartGoogleLogin(proxy);
      setActiveSessionId(res.sessionId);
      setLoginStep(res.step);
      setLoginMessage(res.message);

      // Start live polling status
      pollLoginStatus(res.sessionId);
    } catch (err: any) {
      setLoginStep("FAILED");
      setLoginError(err?.message || String(err));
      setLoginMessage("Không thể khởi chạy cửa sổ Google Chrome.");
      toast.error("Lỗi khởi chạy Chrome", err?.message || String(err));
    }
  };

  // Poll login status from Go backend
  const pollLoginStatus = async (sessionId: string) => {
    try {
      const status = await GetGoogleLoginStatus(sessionId);
      setLoginStep(status.step);
      setLoginMessage(status.message);

      if (status.step === "COMPLETED") {
        toast.success("Thành công", "Tài khoản Google đã được thêm vào hệ thống!");
        fetchAccounts();
        return;
      }

      if (status.step === "FAILED" || status.step === "CANCELLED") {
        if (status.error) setLoginError(status.error);
        return;
      }

      // Continue polling every 1200ms
      pollTimerRef.current = setTimeout(() => pollLoginStatus(sessionId), 1200);
    } catch {
      // Keep polling on transient errors
      pollTimerRef.current = setTimeout(() => pollLoginStatus(sessionId), 1500);
    }
  };

  const handleCancelLogin = async () => {
    if (activeSessionId) {
      try {
        await CancelGoogleLogin(activeSessionId);
      } catch {}
    }
    if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
    setIsLoginModalOpen(false);
    setActiveSessionId(null);
  };

  const handleCloseLoginModal = () => {
    if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
    setIsLoginModalOpen(false);
    setActiveSessionId(null);
    fetchAccounts();
  };

  // Handler: Toggle Active / Disabled (persisted in SQLite)
  const handleToggleActive = async (id: string, active: boolean) => {
    try {
      // Optimistic update
      setAccounts((prev) =>
        prev.map((acc) =>
          acc.id === id ? { ...acc, status: active ? "ACTIVE" : "DISABLED" } : acc
        )
      );

      await ToggleGoogleAccount(id, active);
      toast.success(
        active ? "Đã bật tài khoản" : "Đã tắt tài khoản",
        `Tài khoản đã được chuyển sang trạng thái ${active ? "HOẠT ĐỘNG" : "ĐÃ TẮT"}.`
      );
    } catch (err: any) {
      toast.error("Không thể thay đổi trạng thái", err?.message || String(err));
      fetchAccounts();
    }
  };

  // Track which account is actively refreshing for realtime UI feedback
  const [refreshingAccountId, setRefreshingAccountId] = useState<string | null>(null);

  // Handler: Refresh Single Account
  const handleRefreshAccount = async (id: string) => {
    setRefreshingAccountId(id);
    try {
      const updated = await RefreshGoogleAccount(id);
      if (updated) {
        setAccounts((prev) => prev.map((item) => (item.id === id ? updated : item)));
      }
      toast.success("Làm mới phiên thành công", "Cookie/token đã được cập nhật; quota sẽ tự đồng bộ theo chu kỳ an toàn.");
    } catch (err: any) {
      toast.error("Làm mới thất bại", err?.message || String(err));
      fetchAccounts();
    } finally {
      setRefreshingAccountId(null);
    }
  };

  // Handler: Refresh All Accounts sequentially with live realtime feedback
  const handleRefreshAll = async () => {
    const activeList = accounts.filter((a) => a.status === "ACTIVE");
    if (activeList.length === 0) {
      toast.info("Không có tài khoản", "Không có tài khoản nào đang hoạt động để làm mới.");
      return;
    }

    let successCount = 0;
    let failCount = 0;

    for (let i = 0; i < activeList.length; i++) {
      const acc = activeList[i];
      setRefreshingAccountId(acc.id);
      try {
        const updated = await RefreshGoogleAccount(acc.id);
        if (updated) {
          setAccounts((prev) => prev.map((item) => (item.id === acc.id ? updated : item)));
        }
        successCount++;
      } catch (err: any) {
        failCount++;
        console.error(`Lỗi làm mới ${acc.email}:`, err);
      }
    }

    setRefreshingAccountId(null);
    fetchAccounts();

    if (failCount === 0) {
      toast.success("Làm mới tất cả thành công", `Đã cập nhật realtime toàn bộ ${successCount} tài khoản!`);
    } else {
      toast.warning("Làm mới hoàn tất", `Đã cập nhật ${successCount}/${activeList.length} tài khoản (${failCount} lỗi).`);
    }
  };

  // Handler: Open Dedicated Chrome Browser
  const handleOpenAccountBrowser = async (id: string) => {
    try {
      await OpenGoogleAccountBrowser(id);
      toast.success("Đang mở Chrome", "Cửa sổ Chrome độc lập của tài khoản đang được mở.");
    } catch (err: any) {
      toast.error("Không thể mở Chrome", err?.message || String(err));
    }
  };

  // Handler: Delete Account
  const handleConfirmDelete = async () => {
    if (!deleteTargetId) return;
    setIsDeleting(true);
    try {
      await DeleteGoogleAccount(deleteTargetId);
      toast.success("Đã xóa tài khoản", "Tài khoản và profile lưu trữ đã được dọn dẹp.");
      setDeleteTargetId(null);
      fetchAccounts();
    } catch (err: any) {
      toast.error("Lỗi khi xóa", err?.message || String(err));
    } finally {
      setIsDeleting(false);
    }
  };

  // Handler: Test Account Connectivity & Session
  const handleTestAccount = async (id: string) => {
    try {
      toast.info("Đang kiểm tra", "Đang gửi yêu cầu kiểm tra phiên đăng nhập qua Proxy/Mạng...");
      const res = await TestGoogleAccount(id);
      if (res && res.success) {
        toast.success(
          "Phiên làm việc hợp lệ",
          `${res.message || "Tài khoản kết nối tốt!"} (Độ trễ: ${res.latencyMs}ms)`
        );
      } else {
        toast.error(
          "Phiên làm việc không hợp lệ",
          res?.message || "Phiên đăng nhập đã hết hạn hoặc không kết nối được."
        );
      }
      fetchAccounts();
    } catch (err: any) {
      toast.error("Kiểm tra thất bại", err?.message || String(err));
    }
  };

  // Handler: Save Proxy
  const handleSaveProxy = async (accountId: string, proxy: string) => {
    try {
      await SaveGoogleAccountProxy(accountId, proxy);
      toast.success("Đã lưu Proxy", proxy ? "Proxy đã được lưu an toàn." : "Đã chuyển về kết nối trực tiếp.");
      fetchAccounts();
    } catch (err: any) {
      toast.error("Lỗi khi lưu Proxy", err?.message || String(err));
    }
  };

  const handleOpenProxyModal = async (account: models.GoogleAccountResponse) => {
    setProxyTargetAccount(account);
    setProxyEditorValue("");
    try {
      const current = await GetGoogleAccountProxy(account.id);
      setProxyEditorValue(current || "");
    } catch (err: any) {
      setProxyTargetAccount(null);
      toast.error("Không thể tải Proxy", err?.message || String(err));
    }
  };

  // Handler: Export Accounts Backup
  const handleExportBackup = async () => {
    const password = window.prompt(`Đặt mật khẩu backup (tối thiểu ${MIN_PASSWORD_LENGTH} ký tự):`);
    if (!password) return;
    if (password.length < MIN_PASSWORD_LENGTH) {
      toast.error("Mật khẩu quá ngắn", `Mật khẩu backup phải có ít nhất ${MIN_PASSWORD_LENGTH} ký tự.`);
      return;
    }
    setIsExporting(true);
    try {
      const res = await ExportAccountsBackupDialog(password);
      if (res) {
        const filename = res.filePath.split(/[\\/]/).pop();
        toast.success(
          "Sao lưu thành công",
          `Đã sao lưu ${res.accountCount} tài khoản (${res.profileCount} profiles) vào file ${filename} (${Math.round(res.sizeBytes / 1024)} KB)!`
        );
      }
    } catch (err: any) {
      toast.error("Lỗi xuất sao lưu", err?.message || String(err));
    } finally {
      setIsExporting(false);
    }
  };

  // Handler: Import G-Labs Backup
  const handleImportBackup = async () => {
    const password = window.prompt("Nhập mật khẩu backup nếu file được xuất từ Gateway Manager (bỏ trống với backup G-Labs cũ):");
    if (password === null) return;
    setIsImporting(true);
    try {
      const res = await ImportAccountsBackupDialog(password);
      if (res) {
        const message = `Đã nạp ${res.accountsRestored} tài khoản và ${res.profilesRestored} hồ sơ Chrome.`;
        if (res.success) {
          toast.success("Nạp sao lưu thành công", message);
        } else {
          toast.warning("Nạp sao lưu có cảnh báo", `${message} Một số mục không hợp lệ đã được bỏ qua.`);
        }
        await fetchAccounts();
      }
    } catch (err: any) {
      toast.error("Lỗi nạp sao lưu", err?.message || String(err));
    } finally {
      setIsImporting(false);
    }
  };

  // Handler: Purge Profiles Cache
  const handlePurgeCaches = async () => {
    setIsPurging(true);
    try {
      const res = await PurgeGoogleAccountCaches();
      if (res) {
        toast.success("Dọn rác hoàn tất", res.message);
      }
    } catch (err: any) {
      toast.error("Lỗi dọn rác", err?.message || String(err));
    } finally {
      setIsPurging(false);
    }
  };

  return (
    <div className="flex flex-col w-full space-y-4">
      {/* Account Table Component */}
      <AccountTable
        accounts={accounts}
        isLoading={isLoading}
        onRefreshAll={handleRefreshAll}
        onSyncLiveMetrics={handleSyncLiveMetrics}
        onAddAccount={() => setIsAddAccountModalOpen(true)}
        onOpenBulkAdd={() => setIsBulkAddModalOpen(true)}
        onExportBackup={handleExportBackup}
        onImportBackup={handleImportBackup}
        onPurgeCaches={handlePurgeCaches}
        isPurging={isPurging}
        isExporting={isExporting}
        isImporting={isImporting}
        onToggleActive={handleToggleActive}
        onRefreshAccount={handleRefreshAccount}
        onTestAccount={handleTestAccount}
        onOpenBrowser={handleOpenAccountBrowser}
        onDeleteAccount={(id) => setDeleteTargetId(id)}
        onOpenProxyModal={handleOpenProxyModal}
        liveMetricsByAccount={liveMetricsByAccount}
        isLiveRefreshing={isLiveRefreshing}
        refreshingAccountId={refreshingAccountId}
      />

      {/* Add Account Modal (Chrome Login + Manual Cookie) */}
      <AddAccountModal
        isOpen={isAddAccountModalOpen}
        onClose={() => setIsAddAccountModalOpen(false)}
        onStartChromeLogin={(proxy) => handleStartAddAccount(proxy)}
        onSuccessManual={fetchAccounts}
      />

      {/* Bulk Add Accounts Modal */}
      <BulkAddModal
        isOpen={isBulkAddModalOpen}
        onClose={() => setIsBulkAddModalOpen(false)}
        onSuccess={fetchAccounts}
      />

      {/* Live Chrome Login Progress Modal */}
      <LoginProgressModal
        isOpen={isLoginModalOpen}
        step={loginStep}
        message={loginMessage}
        error={loginError}
        onCancel={handleCancelLogin}
        onClose={handleCloseLoginModal}
      />

      {/* Delete Confirmation Dialog */}
      <ActionConfirmDialog
        isOpen={Boolean(deleteTargetId)}
        title="Xóa tài khoản Google?"
        description="Hành động này sẽ xóa toàn bộ cookie, token và thư mục profile trình duyệt lưu trữ trên đĩa. Bạn sẽ cần đăng nhập lại nếu muốn sử dụng tiếp."
        confirmLabel="Xóa vĩnh viễn"
        cancelLabel="Hủy"
        isDestructive={true}
        isLoading={isDeleting}
        onConfirm={handleConfirmDelete}
        onCancel={() => setDeleteTargetId(null)}
      />

      {/* Proxy Settings Modal */}
      <ProxyModal
        isOpen={Boolean(proxyTargetAccount)}
        account={proxyTargetAccount}
        currentProxy={proxyEditorValue}
        onClose={() => {
          setProxyTargetAccount(null);
          setProxyEditorValue("");
        }}
        onSave={handleSaveProxy}
      />
    </div>
  );
}
