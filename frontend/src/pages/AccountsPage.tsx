import React, { useEffect, useState, useRef } from "react";
import { models } from "@/../wailsjs/go/models";
import {
  ListGoogleAccounts,
  StartGoogleLogin,
  GetGoogleLoginStatus,
  CancelGoogleLogin,
  DeleteGoogleAccount,
  RefreshGoogleAccount,
  RefreshAllGoogleAccounts,
  ToggleGoogleAccount,
  OpenGoogleAccountBrowser,
  ImportAccountsBackupDialog,
  ExportAccountsBackupDialog,
  SaveGoogleAccountProxy,
  UpdateGoogleAccountFeatures,
  BulkUpdateGoogleAccountFeatures,
} from "@/../wailsjs/go/main/App";
import { ActionConfirmDialog } from "@/components/common/ActionConfirmDialog";
import { LoginProgressModal } from "@/components/accounts/LoginProgressModal";
import { AccountTable } from "@/components/accounts/AccountTable";
import { ProxyModal } from "@/components/accounts/ProxyModal";
import { AddAccountModal } from "@/components/accounts/AddAccountModal";
import { BulkAddModal } from "@/components/accounts/BulkAddModal";
import { toast } from "@/lib/toast";

export function AccountsPage() {
  const [accounts, setAccounts] = useState<models.GoogleAccountResponse[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isImporting, setIsImporting] = useState(false);
  const [isExporting, setIsExporting] = useState(false);

  // Modals state
  const [isAddAccountModalOpen, setIsAddAccountModalOpen] = useState(false);
  const [isBulkAddModalOpen, setIsBulkAddModalOpen] = useState(false);

  // Proxies stored in localStorage for fast, local persistence per account
  const [proxies, setProxies] = useState<Record<string, string>>(() => {
    try {
      const stored = localStorage.getItem("dezuxk_account_proxies");
      return stored ? JSON.parse(stored) : {};
    } catch {
      return {};
    }
  });

  // Add Account via Real Chrome Login state
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  const [loginStep, setLoginStep] = useState<string>("INITIALIZING");
  const [loginMessage, setLoginMessage] = useState<string>("");
  const [loginError, setLoginError] = useState<string | undefined>();
  const [isLoginModalOpen, setIsLoginModalOpen] = useState(false);
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Delete confirmation modal state
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  // Proxy modal state
  const [proxyTargetAccount, setProxyTargetAccount] = useState<models.GoogleAccountResponse | null>(null);

  const fetchAccounts = async () => {
    try {
      const data = await ListGoogleAccounts();
      setAccounts(data || []);
      // Synchronize database proxies into local state if empty
      if (data && data.length > 0) {
        setProxies((prev) => {
          const updated = { ...prev };
          data.forEach((a) => {
            if (a.proxy && !updated[a.id]) {
              updated[a.id] = a.proxy;
            }
          });
          return updated;
        });
      }
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
    };
  }, []);

  // Handler: Start Interactive Real Chrome Login
  const handleStartAddAccount = async (service: "gemini" | "flow" = "flow") => {
    try {
      setIsLoginModalOpen(true);
      setLoginStep("INITIALIZING");
      setLoginMessage("Đang quét tìm Google Chrome thật trên máy tính của bạn...");
      setLoginError(undefined);

      const res = await StartGoogleLogin(service);
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

  // Handler: Toggle Image feature for single account (Persist to SQLite)
  const handleToggleImage = async (id: string, imageEnabled: boolean) => {
    const acc = accounts.find((a) => a.id === id);
    if (!acc) return;
    const videoEnabled = acc.videoEnabled ?? true;

    // Optimistic update
    setAccounts((prev) =>
      prev.map((a) => (a.id === id ? { ...a, imageEnabled } : a))
    );

    try {
      await UpdateGoogleAccountFeatures(id, imageEnabled, videoEnabled);
    } catch (err: any) {
      toast.error("Không thể lưu cài đặt Ảnh", err?.message || String(err));
      fetchAccounts();
    }
  };

  // Handler: Toggle Video feature for single account (Persist to SQLite)
  const handleToggleVideo = async (id: string, videoEnabled: boolean) => {
    const acc = accounts.find((a) => a.id === id);
    if (!acc) return;
    const imageEnabled = acc.imageEnabled ?? true;

    // Optimistic update
    setAccounts((prev) =>
      prev.map((a) => (a.id === id ? { ...a, videoEnabled } : a))
    );

    try {
      await UpdateGoogleAccountFeatures(id, imageEnabled, videoEnabled);
    } catch (err: any) {
      toast.error("Không thể lưu cài đặt Video", err?.message || String(err));
      fetchAccounts();
    }
  };

  // Handler: Bulk toggle feature for all accounts (Persist to SQLite)
  const handleBulkToggleFeature = async (feature: "image" | "video", enabled: boolean) => {
    // Optimistic update
    setAccounts((prev) =>
      prev.map((a) =>
        feature === "image" ? { ...a, imageEnabled: enabled } : { ...a, videoEnabled: enabled }
      )
    );

    try {
      await BulkUpdateGoogleAccountFeatures(feature, enabled);
      toast.success(
        `Đã ${enabled ? "bật" : "tắt"} tạo ${feature === "image" ? "ảnh" : "video"} cho tất cả tài khoản`
      );
    } catch (err: any) {
      toast.error("Lỗi cập nhật hàng loạt", err?.message || String(err));
      fetchAccounts();
    }
  };

  // Handler: Refresh Single Account
  const handleRefreshAccount = async (id: string) => {
    try {
      await RefreshGoogleAccount(id);
      toast.success("Làm mới thành công", "Session token đã được cập nhật qua Chrome headless!");
      fetchAccounts();
    } catch (err: any) {
      toast.error("Làm mới thất bại", err?.message || String(err));
    }
  };

  // Handler: Refresh All Accounts
  const handleRefreshAll = async () => {
    try {
      await RefreshAllGoogleAccounts();
      toast.success("Làm mới tất cả", "Đã cập nhật phiên cho toàn bộ các tài khoản đang hoạt động!");
      fetchAccounts();
    } catch (err: any) {
      toast.error("Lỗi làm mới hàng loạt", err?.message || String(err));
      fetchAccounts();
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

  // Handler: Save Proxy
  const handleSaveProxy = async (accountId: string, proxy: string) => {
    const next = { ...proxies, [accountId]: proxy };
    setProxies(next);
    localStorage.setItem("dezuxk_account_proxies", JSON.stringify(next));

    try {
      await SaveGoogleAccountProxy(accountId, proxy);
    } catch {}

    toast.success("Đã lưu Proxy", proxy ? `Proxy đã được lưu: ${proxy}` : "Đã chuyển về kết nối trực tiếp.");
  };

  // Handler: Export Accounts Backup
  const handleExportBackup = async () => {
    setIsExporting(true);
    try {
      const res = await ExportAccountsBackupDialog();
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
    setIsImporting(true);
    try {
      const res = await ImportAccountsBackupDialog();
      if (res) {
        toast.success(
          "Nạp sao lưu thành công",
          `Đã nạp thành công ${res.accountsRestored} tài khoản và ${res.profilesRestored} hồ sơ Chrome!`
        );
        await fetchAccounts();
      }
    } catch (err: any) {
      toast.error("Lỗi nạp sao lưu", err?.message || String(err));
    } finally {
      setIsImporting(false);
    }
  };

  return (
    <div className="flex flex-col w-full space-y-4">
      {/* Account Table Component */}
      <AccountTable
        accounts={accounts}
        isLoading={isLoading}
        onRefreshAll={handleRefreshAll}
        onAddAccount={() => setIsAddAccountModalOpen(true)}
        onOpenBulkAdd={() => setIsBulkAddModalOpen(true)}
        onExportBackup={handleExportBackup}
        onImportBackup={handleImportBackup}
        isExporting={isExporting}
        isImporting={isImporting}
        onToggleActive={handleToggleActive}
        onToggleImage={handleToggleImage}
        onToggleVideo={handleToggleVideo}
        onBulkToggleFeature={handleBulkToggleFeature}
        onRefreshAccount={handleRefreshAccount}
        onOpenBrowser={handleOpenAccountBrowser}
        onDeleteAccount={(id) => setDeleteTargetId(id)}
        onOpenProxyModal={(acc) => setProxyTargetAccount(acc)}
        proxies={proxies}
      />

      {/* Add Account Modal (Chrome Login + Manual Cookie) */}
      <AddAccountModal
        isOpen={isAddAccountModalOpen}
        onClose={() => setIsAddAccountModalOpen(false)}
        onStartChromeLogin={(service) => handleStartAddAccount(service)}
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
        currentProxy={proxyTargetAccount ? proxies[proxyTargetAccount.id] : ""}
        onClose={() => setProxyTargetAccount(null)}
        onSave={handleSaveProxy}
      />
    </div>
  );
}
