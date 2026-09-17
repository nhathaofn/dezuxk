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
  const [service, setService] = useState("flow,gemini");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [result, setResult] = useState<models.BulkAddResult | null>(null);
  const [showGuide, setShowGuide] = useState(false);

  // Reset states on opening
  React.useEffect(() => {
    if (isOpen) {
      setResult(null);
      setIsSubmitting(false);
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
        defaultTier: "PRO",
        service,
      });

      const res = await BulkAddGoogleAccounts(input);
      setResult(res);

      toast.success(
        "Nhập danh sách hoàn tất",
        `Đã thêm ${res.addedCount} tài khoản, bỏ qua ${res.skippedCount}, lỗi ${res.failedCount}.`
      );

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
      className="bg-[#161922] border-zinc-800 text-zinc-100 max-w-2xl"
    >
      <div className="space-y-4 pt-1">
        {/* Header Icon + Description */}
        <div className="flex items-center justify-between pb-3 border-b border-zinc-800">
          <div className="flex items-center gap-3">
            <div className="flex size-9 items-center justify-center rounded-lg bg-indigo-500/10 text-indigo-400 shrink-0">
              <Users className="size-5" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-zinc-200">
                Thêm hàng loạt tài khoản Google / Flow
              </h4>
              <p className="text-xs text-zinc-400">
                Hỗ trợ định dạng phân cách gạch đứng (|), hai chấm (:), chuỗi Cookie hoặc JSON
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={() => setShowGuide(!showGuide)}
            className="flex items-center gap-1 text-xs text-indigo-400 hover:text-indigo-300 transition-colors cursor-pointer"
          >
            <HelpCircle className="size-3.5" />
            <span>{showGuide ? "Ẩn cú pháp" : "Xem cú pháp"}</span>
          </button>
        </div>

        {/* Syntax guide accordion */}
        {showGuide && (
          <div className="rounded-lg bg-zinc-900/90 border border-zinc-700/80 p-3 text-xs space-y-2 text-zinc-300">
            <div className="flex items-center gap-1.5 font-semibold text-indigo-300">
              <Info className="size-3.5" />
              <span>Các định dạng được hệ thống tự động nhận diện:</span>
            </div>
            <ul className="list-disc list-inside space-y-1 font-mono text-[11px] text-zinc-400">
              <li>
                <span className="text-zinc-200">email|password</span> (vd: user1@gmail.com|Matkhau123)
              </li>
              <li>
                <span className="text-zinc-200">email|password|recovery_email</span>
              </li>
              <li>
                <span className="text-zinc-200">email|password|recovery|proxy</span> (vd: user@gmail.com|pass|rec@mail.com|103.14.22.1:8080)
              </li>
              <li>
                <span className="text-zinc-200">email:password</span> hoặc <span className="text-zinc-200">email:password:recovery:proxy</span>
              </li>
              <li>
                <span className="text-zinc-200">Chuỗi cookie Google</span> (chứa __Secure-1PSID=...)
              </li>
            </ul>
          </div>
        )}

        {/* Textarea Input */}
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <label className="text-xs font-semibold text-zinc-300">
              Danh sách tài khoản (mỗi tài khoản 1 dòng)
            </label>
            <span className="text-xs text-zinc-400 font-mono">
              {lineCount} dòng hợp lệ
            </span>
          </div>

          <textarea
            value={rawText}
            onChange={(e) => setRawText(e.target.value)}
            disabled={isSubmitting}
            placeholder={`user1@gmail.com|MatKhau123\nuser2@gmail.com|MatKhau456|backup2@gmail.com\nuser3@gmail.com|MatKhau789|backup3@gmail.com|http://103.1.2.3:8080`}
            rows={8}
            className="w-full rounded-lg bg-zinc-900/90 border border-zinc-700 p-3 text-xs font-mono text-zinc-100 placeholder:text-zinc-600 focus:outline-hidden focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-all resize-y"
          />
        </div>

        {/* Configurations */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
          <div>
            <label className="text-xs font-medium text-zinc-300 block mb-1">
              Proxy mặc định (cho các dòng không có proxy riêng):
            </label>
            <Input
              value={defaultProxy}
              onChange={(e) => setDefaultProxy(e.target.value)}
              disabled={isSubmitting}
              placeholder="vd: http://103.1.2.3:8080 hoặc ip:port:user:pass"
              className="bg-zinc-900/80 border-zinc-700 text-zinc-100 text-xs font-mono placeholder:text-zinc-600"
            />
          </div>

          <div className="flex flex-col justify-end">
            <label className="flex items-center gap-2 cursor-pointer select-none text-xs text-zinc-300 py-2">
              <input
                type="checkbox"
                checked={skipExisting}
                onChange={(e) => setSkipExisting(e.target.checked)}
                disabled={isSubmitting}
                className="size-4 rounded border-zinc-700 bg-zinc-900 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
              />
              <span>Bỏ qua tài khoản đã tồn tại trong cơ sở dữ liệu</span>
            </label>
          </div>
        </div>

        {/* Results summary view */}
        {result && (
          <div className="rounded-lg bg-zinc-900 border border-zinc-800 p-3 space-y-2 text-xs">
            <div className="flex items-center justify-between font-semibold">
              <span className="text-zinc-300">Kết quả xử lý:</span>
              <div className="flex items-center gap-3">
                <span className="text-emerald-400 font-mono">
                  + {result.addedCount} thành công
                </span>
                <span className="text-amber-400 font-mono">
                  ~ {result.skippedCount} bỏ qua
                </span>
                {result.failedCount > 0 && (
                  <span className="text-rose-400 font-mono">
                    ! {result.failedCount} lỗi
                  </span>
                )}
              </div>
            </div>

            <div className="max-h-36 overflow-y-auto space-y-1 divide-y divide-zinc-800/60 font-mono text-[11px] pr-1">
              {result.items.map((it, idx) => (
                <div key={idx} className="flex items-center justify-between py-1">
                  <div className="flex items-center gap-2 truncate min-w-0">
                    {it.status === "ADDED" && <CheckCircle2 className="size-3 text-emerald-400 shrink-0" />}
                    {it.status === "SKIPPED" && <AlertCircle className="size-3 text-amber-400 shrink-0" />}
                    {it.status === "ERROR" && <AlertCircle className="size-3 text-rose-400 shrink-0" />}
                    <span className="truncate text-zinc-200">{it.email}</span>
                  </div>
                  <span
                    className={`shrink-0 ml-2 text-[10px] ${
                      it.status === "ADDED"
                        ? "text-emerald-400"
                        : it.status === "SKIPPED"
                        ? "text-amber-400"
                        : "text-rose-400"
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
        <div className="flex items-center justify-between border-t border-zinc-800 pt-4">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              setRawText("");
              setResult(null);
            }}
            disabled={isSubmitting || (!rawText && !result)}
            className="text-xs text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800"
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
              className="text-xs border-zinc-700 bg-zinc-800 text-zinc-200 hover:bg-zinc-700"
            >
              {result ? "Đóng" : "Hủy"}
            </Button>

            <Button
              type="button"
              size="sm"
              onClick={handleSubmit}
              disabled={isSubmitting || lineCount === 0}
              className="text-xs bg-indigo-600 hover:bg-indigo-500 text-white font-semibold cursor-pointer"
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
