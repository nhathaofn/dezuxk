import React, { useState, useEffect } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { Alert } from "@/components/ui/alert";
import { Dialog } from "@/components/ui/dialog";
import { getSettings, updateGatewayPort, changePassword } from "@/lib/api/settings";
import { toast } from "@/lib/toast";
import { Server, Lock, Loader2 } from "lucide-react";

export function SettingsPage() {
  // Current active port
  const [port, setPort] = useState<number>(8080);

  // Port Modal state
  const [isPortModalOpen, setIsPortModalOpen] = useState(false);
  const [tempPort, setTempPort] = useState<number>(8080);
  const [isSavingPort, setIsSavingPort] = useState(false);
  const [portError, setPortError] = useState<string | null>(null);

  // Password Modal state
  const [isPasswordModalOpen, setIsPasswordModalOpen] = useState(false);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isChangingPass, setIsChangingPass] = useState(false);
  const [passError, setPassError] = useState<string | null>(null);

  useEffect(() => {
    getSettings().then((settings) => {
      if (settings?.gatewayPort) {
        setPort(settings.gatewayPort);
        setTempPort(settings.gatewayPort);
      }
    });
  }, []);

  const handleOpenPortModal = () => {
    setPortError(null);
    setTempPort(port);
    setIsPortModalOpen(true);
  };

  const handleSavePort = async (e: React.FormEvent) => {
    e.preventDefault();
    setPortError(null);

    const portNum = Number(tempPort);
    if (isNaN(portNum) || portNum < 1024 || portNum > 65535) {
      setPortError("Vui lòng nhập cổng hợp lệ (từ 1024 đến 65535).");
      return;
    }

    setIsSavingPort(true);
    try {
      await updateGatewayPort(portNum);
      setPort(portNum);
      toast.success("Cập nhật cổng thành công", `Gateway đã chuyển sang cổng ${portNum}`);
      setIsPortModalOpen(false);
    } catch (err: any) {
      const msg = err?.message || "Lỗi lưu cấu hình cổng.";
      setPortError(msg);
      toast.error("Lỗi cập nhật cổng", msg);
    } finally {
      setIsSavingPort(false);
    }
  };

  const handleOpenPasswordModal = () => {
    setPassError(null);
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
    setIsPasswordModalOpen(true);
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setPassError(null);

    if (!currentPassword) {
      setPassError("Vui lòng nhập mật khẩu hiện tại.");
      return;
    }

    if (newPassword.length < 4) {
      setPassError("Mật khẩu mới phải có ít nhất 4 ký tự.");
      return;
    }

    if (newPassword !== confirmPassword) {
      setPassError("Mật khẩu xác nhận không trùng khớp.");
      return;
    }

    setIsChangingPass(true);
    try {
      await changePassword(currentPassword, newPassword);
      toast.success("Đổi mật khẩu thành công", "Mật khẩu quản trị mới đã được cập nhật.");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setIsPasswordModalOpen(false);
    } catch (err: any) {
      const msg = err?.message || "Không thể đổi mật khẩu. Vui lòng thử lại.";
      setPassError(msg);
      toast.error("Lỗi đổi mật khẩu", msg);
    } finally {
      setIsChangingPass(false);
    }
  };

  return (
    <div className="flex w-full flex-col items-center justify-center py-6 px-4">
      <div className="w-full max-w-lg space-y-3 text-left">
        {/* Compact Gateway Port Row */}
        <Card className="shadow-xs">
          <CardContent className="p-4 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Server className="size-4" />
              </div>
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold text-foreground">
                  Cổng máy chủ Gateway
                </span>
                <span className="rounded-md border border-border bg-muted px-2 py-0.5 font-mono text-xs font-medium text-foreground">
                  {port}
                </span>
              </div>
            </div>

            <Button
              variant="outline"
              size="sm"
              onClick={handleOpenPortModal}
              className="text-xs font-medium"
            >
              Đổi cổng
            </Button>
          </CardContent>
        </Card>

        {/* Compact Admin Password Row */}
        <Card className="shadow-xs">
          <CardContent className="p-4 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Lock className="size-4" />
              </div>
              <span className="text-sm font-semibold text-foreground">
                Mật khẩu quản trị
              </span>
            </div>

            <Button
              variant="outline"
              size="sm"
              onClick={handleOpenPasswordModal}
              className="text-xs font-medium"
            >
              Đổi mật khẩu
            </Button>
          </CardContent>
        </Card>
      </div>

      {/* Change Port Dialog Modal */}
      <Dialog
        open={isPortModalOpen}
        onClose={() => !isSavingPort && setIsPortModalOpen(false)}
        title="Đổi cổng máy chủ Gateway"
      >
        <div className="space-y-4">
          {portError && <Alert variant="destructive">{portError}</Alert>}

          <form onSubmit={handleSavePort} className="space-y-4">
            <div className="space-y-1.5">
              <label
                htmlFor="new-port"
                className="block text-xs font-medium text-foreground"
              >
                Cổng kết nối mới (1024 - 65535)
              </label>
              <Input
                id="new-port"
                type="number"
                min={1024}
                max={65535}
                value={tempPort}
                onChange={(e) => setTempPort(Number(e.target.value))}
                disabled={isSavingPort}
                autoFocus
                required
              />
            </div>

            <div className="flex items-center justify-end gap-2 pt-2 border-t border-border/80">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={isSavingPort}
                onClick={() => setIsPortModalOpen(false)}
                className="text-xs"
              >
                Hủy
              </Button>

              <Button
                type="submit"
                size="sm"
                disabled={isSavingPort}
                className="gap-1.5 text-xs font-medium"
              >
                {isSavingPort ? (
                  <>
                    <Loader2 className="size-3.5 animate-spin" />
                    <span>Đang lưu...</span>
                  </>
                ) : (
                  <span>Lưu thay đổi</span>
                )}
              </Button>
            </div>
          </form>
        </div>
      </Dialog>

      {/* Change Password Dialog Modal */}
      <Dialog
        open={isPasswordModalOpen}
        onClose={() => !isChangingPass && setIsPasswordModalOpen(false)}
        title="Đổi mật khẩu quản trị"
      >
        <div className="space-y-4">
          {passError && <Alert variant="destructive">{passError}</Alert>}

          <form onSubmit={handleChangePassword} className="space-y-4">
            <div className="space-y-1.5">
              <label
                htmlFor="curr-pass"
                className="block text-xs font-medium text-foreground"
              >
                Mật khẩu hiện tại
              </label>
              <PasswordInput
                id="curr-pass"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                placeholder="Nhập mật khẩu hiện tại"
                disabled={isChangingPass}
                autoFocus
                required
              />
            </div>

            <div className="space-y-1.5">
              <label
                htmlFor="new-pass"
                className="block text-xs font-medium text-foreground"
              >
                Mật khẩu mới
              </label>
              <PasswordInput
                id="new-pass"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder="Tối thiểu 4 ký tự"
                disabled={isChangingPass}
                required
              />
            </div>

            <div className="space-y-1.5">
              <label
                htmlFor="confirm-pass"
                className="block text-xs font-medium text-foreground"
              >
                Xác nhận mật khẩu mới
              </label>
              <PasswordInput
                id="confirm-pass"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Nhập lại mật khẩu mới"
                disabled={isChangingPass}
                required
              />
            </div>

            <div className="flex items-center justify-end gap-2 pt-2 border-t border-border/80">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={isChangingPass}
                onClick={() => setIsPasswordModalOpen(false)}
                className="text-xs"
              >
                Hủy
              </Button>

              <Button
                type="submit"
                size="sm"
                disabled={isChangingPass}
                className="gap-1.5 text-xs font-medium"
              >
                {isChangingPass ? (
                  <>
                    <Loader2 className="size-3.5 animate-spin" />
                    <span>Đang lưu...</span>
                  </>
                ) : (
                  <span>Lưu thay đổi</span>
                )}
              </Button>
            </div>
          </form>
        </div>
      </Dialog>
    </div>
  );
}
