import React, { useState } from "react";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Users, CheckCircle2, AlertCircle, Loader2, Info, HelpCircle } from "lucide-react";
import { models } from "@/../wailsjs/go/models";
import { BulkAddGoogleAccounts } from "@/../wailsjs/go/main/App";
import { toast } from "@/lib/toast";

interface BulkAddModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

export function BulkAddModal({ isOpen, onClose, onSuccess }: BulkAddModalProps) {
  const [rawText, setRawText] = useState("");
  const [defaultProxy, setDefaultProxy] = useState("");
  const [skipExisting, setSkipExisting] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [result, setResult] = useState<models.BulkAddResult | null>(null);
  const [showGuide, setShowGuide] = useState(false);

  // Reset states on opening
  React.useEffect(() => {
    if (isOpen) {
      setResult(null);
      setIsSubmitting(false);
    } else {
      // Cookie strings are session credentials; do not retain them after the dialog closes.
      setRawText("");
      setDefaultProxy("");
    }
  }, [isOpen]);

  const lines = rawText
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l.length > 0 && !l.startsWith("#") && !l.startsWith("//"));

  const lineCount = lines.length;

  const handleSubmit = async () => {
    if (lineCount === 0) {
      toast.error("Danh sách trống", "Vui lòng dán danh sách tài khoản vào ô nhập liệu.");
      return;
    }

    setIsSubmitting(true);
    setResult(null);

    try {
      const input = new models.BulkAddInput({
        rawList: rawText,
        defaultProxy: defaultProxy.trim(),
        skipExisting,
        defaultTier: "FREE",
        service: "flow,gemini",
      });

      const res = await BulkAddGoogleAccounts(input);
      setResult(res);
      setRawText("");

      const summary = `Đã thêm ${res.addedCount} tài khoản, bỏ qua ${res.skippedCount}, lỗi ${res.failedCount}.`;
      if (res.failedCount > 0 && res.addedCount === 0) {
        toast.error("Nhập danh sách thất bại", summary);
      } else if (res.failedCount > 0) {
        toast.warning("Nhập danh sách có cảnh báo", summary);
      } else {
        toast.success("Nhập danh sách hoàn tất", summary);
      }

      if (res.addedCount > 0) {
        onSuccess();
      }
    } catch (err: any) {
      toast.error("Lỗi nhập danh sách", err?.message || String(err));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog
      open={isOpen}
      onClose={onClose}
      title="Thêm tài khoản theo danh sách"
      className="max-w-2xl"
    >
      <div className="space-y-4 pt-1">
        {/* Header Icon + Description */}
        <div className="flex items-center justify-between pb-3 border-b border-border">
          <div className="flex items-center gap-3">
            <div className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
              <Users className="size-5" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-foreground">
                Thêm hàng loạt tài khoản Google
              </h4>
              <p className="text-xs text-muted-foreground">
                Hỗ trợ định dạng phân cách gạch đứng (|), tab, chuỗi Cookie hoặc JSON
              </p>
              <p className="mt-1 text-[11px] text-amber-700 dark:text-amber-400">
                Tài khoản nhập bằng Cookie sẽ ở trạng thái chờ kiểm tra; hãy bấm Kiểm tra phiên trước khi bật.
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={() => setShowGuide(!showGuide)}
            className="flex items-center gap-1 text-xs text-primary hover:underline transition-colors cursor-pointer"
          >
            <HelpCircle className="size-3.5" />
            <span>{showGuide ? "Ẩn cú pháp" : "Xem cú pháp"}</span>
          </button>
        </div>

        {/* Syntax guide accordion */}
        {showGuide && (
          <div className="rounded-lg border border-border bg-muted/40 p-3 text-xs space-y-2 text-foreground">
            <div className="flex items-center gap-1.5 font-semibold text-primary">
              <Info className="size-3.5" />
              <span>Các định dạng được hệ thống tự động nhận diện:</span>
            </div>
            <ul className="list-disc list-inside space-y-1 font-mono text-[11px] text-muted-foreground">
              <li>
                <span className="text-foreground">email|cookie</span> (Cookie chứa __Secure-1PSID=...)
              </li>
              <li>
                <span className="text-foreground">email|cookie|proxy</span>
              </li>
              <li>
                <span className="text-foreground">JSON</span> với <span className="text-foreground">email</span>, <span className="text-foreground">cookie</span> và <span className="text-foreground">proxy</span> tùy chọn
              </li>
              <li>
                Không nhập mật khẩu Google; hệ thống chỉ nhận phiên Cookie đã đăng nhập
              </li>
            </ul>
          </div>
        )}

        {/* Textarea Input */}
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <label className="text-xs font-semibold text-foreground">
              Danh sách tài khoản (mỗi tài khoản 1 dòng)
            </label>
            <span className="text-xs text-muted-foreground font-mono">
              {lineCount} dòng hợp lệ
            </span>
          </div>

          <textarea
            value={rawText}
            onChange={(e) => setRawText(e.target.value)}
            disabled={isSubmitting}
            placeholder={`user1@gmail.com|__Secure-1PSID=...; SID=...;\nuser2@gmail.com|__Secure-1PSID=...; SID=...;|http://103.1.2.3:8080\n{"email":"user3@gmail.com","cookie":"SID=..."}`}
            rows={8}
            className="w-full rounded-lg bg-background border border-input p-3 text-xs font-mono text-foreground placeholder:text-muted-foreground focus:outline-hidden focus:border-ring focus:ring-1 focus:ring-ring/50 transition-all resize-y"
          />
        </div>

        {/* Configurations */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
          <div>
            <label className="text-xs font-medium text-foreground block mb-1">
              Proxy mặc định (cho các dòng không có proxy riêng):
            </label>
            <Input
              value={defaultProxy}
              onChange={(e) => setDefaultProxy(e.target.value)}
              disabled={isSubmitting}
              placeholder="vd: http://103.1.2.3:8080 hoặc ip:port:user:pass"
              className="bg-background border-input text-foreground text-xs font-mono placeholder:text-muted-foreground"
            />
          </div>

          <div className="flex flex-col justify-end">
            <label className="flex items-center gap-2 cursor-pointer select-none text-xs text-foreground py-2">
              <input
                type="checkbox"
                checked={skipExisting}
                onChange={(e) => setSkipExisting(e.target.checked)}
                disabled={isSubmitting}
                className="size-4 rounded border-input bg-background text-primary focus:ring-ring cursor-pointer"
              />
              <span>Bỏ qua tài khoản đã tồn tại trong cơ sở dữ liệu</span>
            </label>
          </div>
        </div>

        {/* Results summary view */}
        {result && (
          <div className="rounded-lg bg-muted/40 border border-border p-3 space-y-2 text-xs">
            <div className="flex items-center justify-between font-semibold">
              <span className="text-foreground">Kết quả xử lý:</span>
              <div className="flex items-center gap-3">
                <span className="text-emerald-700 dark:text-emerald-400 font-mono">
                  + {result.addedCount} thành công
                </span>
                <span className="text-amber-700 dark:text-amber-400 font-mono">
                  ~ {result.skippedCount} bỏ qua
                </span>
                {result.failedCount > 0 && (
                  <span className="text-rose-700 dark:text-rose-400 font-mono">
                    ! {result.failedCount} lỗi
                  </span>
                )}
              </div>
            </div>

            <div className="max-h-36 overflow-y-auto space-y-1 divide-y divide-border/60 font-mono text-[11px] pr-1">
              {result.items.map((it, idx) => (
                <div key={idx} className="flex items-center justify-between py-1">
                  <div className="flex items-center gap-2 truncate min-w-0">
                    {it.status === "ADDED" && <CheckCircle2 className="size-3 text-emerald-600 dark:text-emerald-400 shrink-0" />}
                    {it.status === "SKIPPED" && <AlertCircle className="size-3 text-amber-600 dark:text-amber-400 shrink-0" />}
                    {it.status === "ERROR" && <AlertCircle className="size-3 text-rose-600 dark:text-rose-400 shrink-0" />}
                    <span className="truncate text-foreground">{it.email}</span>
                  </div>
                  <span
                    className={`shrink-0 ml-2 text-[10px] ${
                      it.status === "ADDED"
                         ? "text-emerald-700 dark:text-emerald-400"
                        : it.status === "SKIPPED"
                         ? "text-amber-700 dark:text-amber-400"
                         : "text-rose-700 dark:text-rose-400"
                    }`}
                  >
                    {it.message}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Action Buttons */}
        <div className="flex items-center justify-between border-t border-border pt-4">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              setRawText("");
              setResult(null);
            }}
            disabled={isSubmitting || (!rawText && !result)}
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            Xóa danh sách
          </Button>

          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onClose}
              disabled={isSubmitting}
              className="text-xs"
            >
              {result ? "Đóng" : "Hủy"}
            </Button>

            <Button
              type="button"
              size="sm"
              onClick={handleSubmit}
              disabled={isSubmitting || lineCount === 0}
              className="text-xs bg-primary hover:bg-primary/90 text-primary-foreground font-semibold cursor-pointer"
            >
              {isSubmitting ? (
                <>
                  <Loader2 className="size-3.5 animate-spin mr-1.5" />
                  Đang thêm ({lineCount})...
                </>
              ) : (
                `Thêm ${lineCount > 0 ? `(${lineCount})` : ""} tài khoản`
              )}
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  );
}
