import React, { useState } from "react";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Network, CheckCircle2, XCircle, Loader2 } from "lucide-react";
import { models } from "@/../wailsjs/go/models";

import { TestGoogleAccountProxy } from "@/../wailsjs/go/main/App";

interface ProxyModalProps {
  isOpen: boolean;
  account: models.GoogleAccountResponse | null;
  currentProxy?: string;
  onClose: () => void;
  onSave: (accountId: string, proxy: string) => Promise<void>;
}

export function ProxyModal({
  isOpen,
  account,
  currentProxy = "",
  onClose,
  onSave,
}: ProxyModalProps) {
  const [proxyValue, setProxyValue] = useState(currentProxy);
  const [isSaving, setIsSaving] = useState(false);
  const [testStatus, setTestStatus] = useState<"idle" | "testing" | "success" | "error">("idle");
  const [testMessage, setTestMessage] = useState("");

  // Sync state when opened
  React.useEffect(() => {
    if (isOpen) {
      setProxyValue(currentProxy || "");
      setTestStatus("idle");
      setTestMessage("");
    } else {
      // Proxy strings may contain credentials; do not retain them after closing.
      setProxyValue("");
      setTestStatus("idle");
      setTestMessage("");
    }
  }, [isOpen, currentProxy]);

  const handleTest = async () => {
    if (!proxyValue.trim()) {
      setTestStatus("error");
      setTestMessage("Vui lòng nhập địa chỉ Proxy để kiểm tra.");
      return;
    }

    setTestStatus("testing");
    setTestMessage("Đang kiểm tra kết nối qua proxy tới máy chủ Google...");

    try {
      const res = await TestGoogleAccountProxy(proxyValue.trim());
      if (res.success) {
        setTestStatus("success");
        setTestMessage(
          res.egressIP
            ? `Proxy hoạt động tốt! Độ trễ: ${res.latencyMs} ms | IP xuất ra: ${res.egressIP}`
            : res.message || `Kết nối thành công (${res.latencyMs} ms)`
        );
      } else {
        setTestStatus("error");
        setTestMessage(res.message || "Không thể kết nối qua Proxy này.");
      }
    } catch (err: any) {
      setTestStatus("error");
      setTestMessage(`Lỗi kiểm tra proxy: ${err?.message || String(err)}`);
    }
  };

  const handleSave = async () => {
    if (!account) return;
    setIsSaving(true);
    try {
      await onSave(account.id, proxyValue.trim());
      onClose();
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Dialog
      open={isOpen}
      onClose={onClose}
      title="Cấu hình Proxy tài khoản"
      className=""
    >
      <div className="space-y-4 pt-1">
        <div className="flex items-center gap-3 pb-3 border-b border-border">
          <div className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
            <Network className="size-4" />
          </div>
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground truncate">Tài khoản áp dụng:</p>
            <p className="text-xs font-semibold text-foreground truncate">{account?.email || "Google Account"}</p>
          </div>
        </div>

        <div>
          <label className="text-xs font-semibold text-foreground block mb-1.5">
            Địa chỉ Proxy (HTTP / SOCKS5)
          </label>
          <Input
            value={proxyValue}
            onChange={(e) => setProxyValue(e.target.value)}
            placeholder="vd: http://103.142.23.11:8080 hoặc http://user:pass@ip:port"
            className="bg-background border-input text-foreground text-xs placeholder:text-muted-foreground font-mono"
          />
          <p className="text-[11px] text-muted-foreground mt-1.5">
            Để trống nếu bạn muốn tài khoản này kết nối mạng trực tiếp (Direct Connection).
          </p>
        </div>

        {testStatus !== "idle" && (
          <div
            className={`flex items-start gap-2 p-3 rounded-lg text-xs ${
              testStatus === "testing"
                ? "bg-muted text-muted-foreground"
                : testStatus === "success"
                ? "bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-300"
                : "bg-rose-500/10 border border-rose-500/20 text-rose-700 dark:text-rose-300"
            }`}
          >
            {testStatus === "testing" && <Loader2 className="size-4 animate-spin shrink-0 mt-0.5" />}
            {testStatus === "success" && <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400 shrink-0 mt-0.5" />}
            {testStatus === "error" && <XCircle className="size-4 text-destructive shrink-0 mt-0.5" />}
            <span>{testMessage}</span>
          </div>
        )}

        <div className="flex items-center justify-between gap-2 border-t border-border pt-4 mt-6">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleTest}
            disabled={testStatus === "testing"}
            className="text-xs"
          >
            Kiểm tra
          </Button>

          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onClose}
              disabled={isSaving}
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              Đóng
            </Button>
            <Button
              type="button"
              size="sm"
              onClick={handleSave}
              disabled={isSaving}
              className="text-xs bg-primary hover:bg-primary/90 text-primary-foreground font-semibold"
            >
              {isSaving ? "Đang lưu..." : "Lưu Proxy"}
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  );
}
