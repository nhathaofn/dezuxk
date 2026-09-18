import React, { useState } from "react";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Globe, Compass, KeyRound, Loader2, Sparkles, AlertCircle } from "lucide-react";
import { models } from "@/../wailsjs/go/models";
import { AddGoogleAccountManual } from "@/../wailsjs/go/main/App";
import { toast } from "@/lib/toast";

interface AddAccountModalProps {
  isOpen: boolean;
  onClose: () => void;
  onStartChromeLogin: (service: "flow" | "gemini", proxy: string) => void;
  onSuccessManual: () => void;
}

export function AddAccountModal({
  isOpen,
  onClose,
  onStartChromeLogin,
  onSuccessManual,
}: AddAccountModalProps) {
  const [tab, setTab] = useState<"chrome" | "cookie">("chrome");
  const [service, setService] = useState<"flow" | "gemini">("flow");
  const [chromeProxy, setChromeProxy] = useState("");

  // Manual Cookie form state
  const [email, setEmail] = useState("");
  const [cookies, setCookies] = useState("");
  const [snlm0e, setSnlm0e] = useState("");
  const [proxy, setProxy] = useState("");
  const [tier, setTier] = useState("PRO");
  const [creditsInput, setCreditsInput] = useState("0");
  const [isSubmitting, setIsSubmitting] = useState(false);

  React.useEffect(() => {
    if (isOpen) {
      setCookies("");
      setSnlm0e("");
      setEmail("");
      setProxy("");
      setChromeProxy("");
      setCreditsInput("0");
      setIsSubmitting(false);
    }
  }, [isOpen]);

  const handleManualSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!cookies.trim()) {
      toast.error("Thiếu Cookie", "Vui lòng dán chuỗi cookie của tài khoản Google.");
      return;
    }

    setIsSubmitting(true);
    try {
      const input = new models.ManualAccountInput({
        email: email.trim(),
        cookies: cookies.trim(),
        snlm0eToken: snlm0e.trim(),
        proxy: proxy.trim(),
        tier,
        credits: parseInt(creditsInput, 10) || 0,
        service,
      });

      await AddGoogleAccountManual(input);
      toast.success("Thành công", "Tài khoản đã được thêm thủ công vào hệ thống!");
      onSuccessManual();
      onClose();
    } catch (err: any) {
      toast.error("Lỗi thêm tài khoản", err?.message || String(err));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog
      open={isOpen}
      onClose={onClose}
      title="Thêm tài khoản Google"
      className="bg-[#161922] border-zinc-800 text-zinc-100 max-w-lg"
    >
      <div className="space-y-4 pt-1">
        {/* Navigation Tabs */}
        <div className="flex border-b border-zinc-800">
          <button
            type="button"
            onClick={() => setTab("chrome")}
            className={`flex items-center gap-2 pb-2.5 px-3 text-xs font-semibold border-b-2 transition-colors cursor-pointer ${
              tab === "chrome"
                ? "border-primary text-primary"
                : "border-transparent text-zinc-400 hover:text-zinc-200"
            }`}
          >
            <Compass className="size-3.5" />
            <span>Đăng nhập Chrome thật</span>
          </button>

          <button
            type="button"
            onClick={() => setTab("cookie")}
            className={`flex items-center gap-2 pb-2.5 px-3 text-xs font-semibold border-b-2 transition-colors cursor-pointer ${
              tab === "cookie"
                ? "border-primary text-primary"
                : "border-transparent text-zinc-400 hover:text-zinc-200"
            }`}
          >
            <KeyRound className="size-3.5" />
            <span>Nhập Cookie thủ công</span>
          </button>
        </div>

        {/* Tab 1: Interactive Chrome Login */}
        {tab === "chrome" && (
          <div className="space-y-4">
            <div className="rounded-lg bg-zinc-900/80 border border-zinc-800 p-3 text-xs text-zinc-300 space-y-2">
              <div className="flex items-center gap-2 font-semibold text-zinc-200">
                <Sparkles className="size-4 text-amber-400 shrink-0" />
                <span>Cơ chế đăng nhập độc lập & an toàn</span>
              </div>
              <p className="text-zinc-400 leading-relaxed text-[11px]">
                Một cửa sổ Google Chrome riêng biệt sẽ mở ra. Bạn chỉ cần thực hiện đăng nhập tài khoản Google của mình. Hệ thống sẽ tự động bắt phiên, trích xuất CSRF Token, lưu trữ Profile và hoàn tất quá trình.
              </p>
            </div>

            <div>
              <label className="text-xs font-semibold text-zinc-300 block mb-1.5">
                Dịch vụ mục tiêu:
              </label>
              <div className="grid grid-cols-2 gap-3">
                <button
                  type="button"
                  onClick={() => setService("flow")}
                  className={`flex flex-col items-start p-3 rounded-lg border text-left transition-all cursor-pointer ${
                    service === "flow"
                      ? "border-primary bg-primary/10 text-zinc-100"
                      : "border-zinc-800 bg-zinc-900/50 text-zinc-400 hover:border-zinc-700 hover:text-zinc-300"
                  }`}
                >
                  <span className="text-xs font-bold text-primary">Flow Google (Tạo video/ảnh)</span>
                  <span className="text-[10px] text-zinc-400 mt-0.5">flow.google.com (Veo & Imagen)</span>
                </button>

                <button
                  type="button"
                  onClick={() => setService("gemini")}
                  className={`flex flex-col items-start p-3 rounded-lg border text-left transition-all cursor-pointer ${
                    service === "gemini"
                      ? "border-primary bg-primary/10 text-zinc-100"
                      : "border-zinc-800 bg-zinc-900/50 text-zinc-400 hover:border-zinc-700 hover:text-zinc-300"
                  }`}
                >
                  <span className="text-xs font-bold text-amber-400">Gemini Web / AI Studio</span>
                  <span className="text-[10px] text-zinc-400 mt-0.5">gemini.google.com (Chat & Multimodal)</span>
                </button>
              </div>
            </div>

            <div>
              <label className="text-xs font-semibold text-zinc-300 block mb-1.5">
                Proxy cho trình duyệt (tùy chọn):
              </label>
              <Input
                value={chromeProxy}
                onChange={(e) => setChromeProxy(e.target.value)}
                placeholder="vd: http://103.14.22.1:8080 hoặc ip:port:user:pass"
                className="bg-zinc-900/80 border-zinc-700 text-zinc-100 text-xs font-mono placeholder:text-zinc-600"
              />
              <p className="text-[11px] text-zinc-400 mt-1">
                Chrome sẽ mở qua Proxy này để đăng nhập, hạn chế bị Google checkpoint vị trí.
              </p>
            </div>

            <div className="flex items-center justify-end gap-2 pt-3 border-t border-zinc-800">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={onClose}
                className="text-xs border-zinc-700 bg-zinc-800 text-zinc-200 hover:bg-zinc-700"
              >
                Hủy
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => {
                  onClose();
                  onStartChromeLogin(service, chromeProxy.trim());
                }}
                className="text-xs bg-primary hover:bg-primary/90 text-primary-foreground font-semibold cursor-pointer"
              >
                <Compass className="size-3.5 mr-1.5" />
                Mở Chrome Đăng Nhập
              </Button>
            </div>
          </div>
        )}

        {/* Tab 2: Manual Cookie Entry */}
        {tab === "cookie" && (
          <form onSubmit={handleManualSubmit} className="space-y-3">
            <div>
              <label className="text-xs font-semibold text-zinc-300 block mb-1">
                Chuỗi Cookie Google <span className="text-rose-400">*</span>
              </label>
              <textarea
                value={cookies}
                onChange={(e) => setCookies(e.target.value)}
                placeholder="Dán chuỗi cookie (chứa __Secure-1PSID=...; SID=...; __Secure-1PSIDTS=...)"
                rows={3}
                required
                className="w-full rounded-lg bg-zinc-900/90 border border-zinc-700 p-2.5 text-xs font-mono text-zinc-100 placeholder:text-zinc-600 focus:outline-hidden focus:border-primary focus:ring-1 focus:ring-primary transition-all resize-y"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs font-medium text-zinc-300 block mb-1">
                  Email (tùy chọn):
                </label>
                <Input
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="vd: myaccount@gmail.com"
                  className="bg-zinc-900/80 border-zinc-700 text-zinc-100 text-xs font-mono placeholder:text-zinc-600"
                />
              </div>

              <div>
                <label className="text-xs font-medium text-zinc-300 block mb-1">
                  Token SNlM0e (tùy chọn):
                </label>
                <Input
                  value={snlm0e}
                  onChange={(e) => setSnlm0e(e.target.value)}
                  placeholder="CSRF Token SNlM0e"
                  className="bg-zinc-900/80 border-zinc-700 text-zinc-100 text-xs font-mono placeholder:text-zinc-600"
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs font-medium text-zinc-300 block mb-1">
                  Proxy riêng (tùy chọn):
                </label>
                <Input
                  value={proxy}
                  onChange={(e) => setProxy(e.target.value)}
                  placeholder="http://103.1.2.3:8080"
                  className="bg-zinc-900/80 border-zinc-700 text-zinc-100 text-xs font-mono placeholder:text-zinc-600"
                />
              </div>

              <div>
                <label className="text-xs font-medium text-zinc-300 block mb-1">
                  Hạng (Tier):
                </label>
                <select
                  value={tier}
                  onChange={(e) => setTier(e.target.value)}
                  className="w-full h-8 rounded-md bg-zinc-900/80 border border-zinc-700 text-zinc-100 text-xs px-2 focus:outline-hidden focus:border-primary"
                >
                  <option value="PRO">PRO</option>
                  <option value="FREE">FREE</option>
                  <option value="ULTRA">ULTRA</option>
                </select>
              </div>

              <div>
                <label className="text-xs font-medium text-zinc-300 block mb-1">
                  Số Tín Dụng Ban Đầu:
                </label>
                <Input
                  type="number"
                  min="0"
                  value={creditsInput}
                  onChange={(e) => setCreditsInput(e.target.value)}
                  placeholder="0"
                  className="bg-zinc-900/80 border-zinc-700 text-zinc-100 text-xs font-mono placeholder:text-zinc-600"
                />
              </div>
            </div>

            <div className="flex items-center justify-end gap-2 pt-3 border-t border-zinc-800">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={onClose}
                disabled={isSubmitting}
                className="text-xs border-zinc-700 bg-zinc-800 text-zinc-200 hover:bg-zinc-700"
              >
                Hủy
              </Button>
              <Button
                type="submit"
                size="sm"
                disabled={isSubmitting || !cookies.trim()}
                className="text-xs bg-primary hover:bg-primary/90 text-primary-foreground font-semibold cursor-pointer"
              >
                {isSubmitting ? (
                  <>
                    <Loader2 className="size-3.5 animate-spin mr-1.5" />
                    Đang lưu...
                  </>
                ) : (
                  "Thêm tài khoản"
                )}
              </Button>
            </div>
          </form>
        )}
      </div>
    </Dialog>
  );
}
