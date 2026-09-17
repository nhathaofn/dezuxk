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
      className="bg-[#161922] border-zinc-800 text-zinc-100"
    >
      <div className="space-y-4 pt-1">
        <div className="flex items-center gap-3 pb-3 border-b border-zinc-800">
          <div className="flex size-9 items-center justify-center rounded-lg bg-amber-500/10 text-amber-400 shrink-0">
            <Network className="size-4" />
          </div>
          <div className="min-w-0">
            <p className="text-xs text-zinc-400 truncate">Tài khoản áp dụng:</p>
            <p className="text-xs font-semibold text-zinc-200 truncate">{account?.email || "Google Account"}</p>
          </div>
        </div>

        <div>
          <label className="text-xs font-semibold text-zinc-300 block mb-1.5">
            Địa chỉ Proxy (HTTP / SOCKS5)
          </label>
          <Input
            value={proxyValue}
            onChange={(e) => setProxyValue(e.target.value)}
            placeholder="vd: http://103.142.23.11:8080 hoặc http://user:pass@ip:port"
            className="bg-zinc-900/80 border-zinc-700 text-zinc-100 text-xs placeholder:text-zinc-500 font-mono"
          />
          <p className="text-[11px] text-zinc-400 mt-1.5">
            Để trống nếu bạn muốn tài khoản này kết nối mạng trực tiếp (Direct Connection).
          </p>
        </div>

        {testStatus !== "idle" && (
          <div
            className={`flex items-start gap-2 p-3 rounded-lg text-xs ${
              testStatus === "testing"
                ? "bg-zinc-800/60 text-zinc-300"
                : testStatus === "success"
                ? "bg-emerald-950/40 border border-emerald-800/40 text-emerald-300"
                : "bg-rose-950/40 border border-rose-800/40 text-rose-300"
            }`}
          >
            {testStatus === "testing" && <Loader2 className="size-4 animate-spin shrink-0 mt-0.5" />}
            {testStatus === "success" && <CheckCircle2 className="size-4 text-emerald-400 shrink-0 mt-0.5" />}
            {testStatus === "error" && <XCircle className="size-4 text-rose-400 shrink-0 mt-0.5" />}
            <span>{testMessage}</span>
          </div>
        )}

        <div className="flex items-center justify-between gap-2 border-t border-zinc-800/80 pt-4 mt-6">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleTest}
            disabled={testStatus === "testing"}
            className="text-xs border-zinc-700 bg-zinc-800 text-zinc-200 hover:bg-zinc-700"
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
              className="text-xs text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800"
            >
              Đóng
            </Button>
            <Button
              type="button"
              size="sm"
              onClick={handleSave}
              disabled={isSaving}
              className="text-xs bg-amber-600 hover:bg-amber-500 text-white font-semibold"
            >
              {isSaving ? "Đang lưu..." : "Lưu Proxy"}
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  );
}
