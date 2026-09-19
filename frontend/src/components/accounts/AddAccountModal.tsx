import React, { useState } from "react";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Compass, KeyRound, Loader2, Sparkles, CheckCircle2, XCircle } from "lucide-react";
import { models } from "@/../wailsjs/go/models";
import { AddGoogleAccountManual, TestGoogleAccountProxy } from "@/../wailsjs/go/main/App";
import { toast } from "@/lib/toast";

interface AddAccountModalProps {
  isOpen: boolean;
  onClose: () => void;
  onStartChromeLogin: (proxy: string) => void;
  onSuccessManual: () => void;
}

export function AddAccountModal({
  isOpen,
  onClose,
  onStartChromeLogin,
  onSuccessManual,
}: AddAccountModalProps) {
  const [tab, setTab] = useState<"chrome" | "cookie">("chrome");
  const [chromeProxy, setChromeProxy] = useState("");
  const [chromeProxyTesting, setChromeProxyTesting] = useState(false);
  const [chromeProxyResult, setChromeProxyResult] = useState<{
    success: boolean;
    message: string;
    latency?: number;
    ip?: string;
  } | null>(null);

  // Manual Cookie form state
  const [email, setEmail] = useState("");
  const [cookies, setCookies] = useState("");
  const [snlm0e, setSnlm0e] = useState("");
  const [proxy, setProxy] = useState("");
  const [manualProxyTesting, setManualProxyTesting] = useState(false);
  const [manualProxyResult, setManualProxyResult] = useState<{
    success: boolean;
    message: string;
    latency?: number;
    ip?: string;
  } | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  React.useEffect(() => {
    if (isOpen) {
      setCookies("");
      setSnlm0e("");
      setEmail("");
      setProxy("");
      setChromeProxy("");
      setChromeProxyResult(null);
      setManualProxyResult(null);
      setIsSubmitting(false);
    } else {
      // Cookie and token fields are credentials; clear them when the dialog closes.
      setCookies("");
      setSnlm0e("");
      setProxy("");
      setChromeProxy("");
      setChromeProxyResult(null);
      setManualProxyResult(null);
    }
  }, [isOpen]);

  const handleTestChromeProxy = async () => {
    if (!chromeProxy.trim()) {
      toast.error("Thiếu Proxy", "Vui lòng nhập địa chỉ Proxy trước khi kiểm tra.");
      return;
    }
    setChromeProxyTesting(true);
    setChromeProxyResult(null);
    try {
      const res = await TestGoogleAccountProxy(chromeProxy.trim());
      setChromeProxyResult({
        success: res.success,
        message: res.message,
        latency: res.latencyMs,
        ip: res.egressIP,
      });
    } catch (err: any) {
      setChromeProxyResult({
        success: false,
        message: err?.message || String(err),
      });
    } finally {
      setChromeProxyTesting(false);
    }
  };

  const handleTestManualProxy = async () => {
    if (!proxy.trim()) {
      toast.error("Thiếu Proxy", "Vui lòng nhập địa chỉ Proxy trước khi kiểm tra.");
      return;
    }
    setManualProxyTesting(true);
    setManualProxyResult(null);
    try {
      const res = await TestGoogleAccountProxy(proxy.trim());
      setManualProxyResult({
        success: res.success,
        message: res.message,
        latency: res.latencyMs,
        ip: res.egressIP,
      });
    } catch (err: any) {
      setManualProxyResult({
        success: false,
        message: err?.message || String(err),
      });
    } finally {
      setManualProxyTesting(false);
    }
  };

  const handleManualSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const cleanCookies = cookies.trim();
    if (!cleanCookies) {
      toast.error("Thiếu Cookie", "Vui lòng dán chuỗi cookie của tài khoản Google.");
      return;
    }

    if (!cleanCookies.includes("__Secure-1PSID=") && !cleanCookies.includes("SID=")) {
      toast.error(
        "Cookie không hợp lệ",
        "Chuỗi cookie phải chứa ít nhất __Secure-1PSID=... hoặc SID=... của Google."
      );
      return;
    }

    setIsSubmitting(true);
    try {
      const input = new models.ManualAccountInput({
        email: email.trim(),
        cookies: cleanCookies,
        snlm0eToken: snlm0e.trim(),
        proxy: proxy.trim(),
        // Quota and tier are read live after import; legacy fields stay neutral.
        tier: "FREE",
        credits: 0,
        service: "flow,gemini",
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
      className="max-w-lg"
    >
      <div className="space-y-4 pt-1">
        {/* Navigation Tabs */}
        <div className="flex border-b border-border">
          <button
            type="button"
            onClick={() => setTab("chrome")}
            className={`flex items-center gap-2 pb-2.5 px-3 text-xs font-semibold border-b-2 transition-colors cursor-pointer ${
              tab === "chrome"
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
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
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            <KeyRound className="size-3.5" />
            <span>Nhập Cookie thủ công</span>
          </button>
        </div>

        {/* Tab 1: Interactive Chrome Login */}
        {tab === "chrome" && (
          <div className="space-y-4">
            <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 text-xs text-foreground space-y-2">
              <div className="flex items-center gap-2 font-semibold">
                <Sparkles className="size-4 text-primary shrink-0" />
                <span>Đăng nhập một lần · đồng bộ Flow + Gemini</span>
              </div>
              <p className="text-muted-foreground leading-relaxed text-[11px]">
                Một cửa sổ Chrome cô lập sẽ mở ra. Sau khi đăng nhập Google, hệ thống tự lưu cùng một profile/cookie cho cả Flow (Veo & Imagen) và Gemini (Chat & Multimodal), không cần chọn dịch vụ.
              </p>
            </div>

            <div>
              <div className="flex items-center justify-between mb-1.5">
                <label className="text-xs font-semibold text-foreground">
                  Proxy cho trình duyệt (tùy chọn):
                </label>
                {chromeProxy.trim() && (
                  <button
                    type="button"
                    onClick={handleTestChromeProxy}
                    disabled={chromeProxyTesting}
                    className="text-[11px] text-primary hover:underline font-medium cursor-pointer flex items-center gap-1"
                  >
                    {chromeProxyTesting && <Loader2 className="size-2.5 animate-spin" />}
                    <span>Kiểm tra Proxy</span>
                  </button>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Input
                  value={chromeProxy}
                  onChange={(e) => {
                    setChromeProxy(e.target.value);
                    setChromeProxyResult(null);
                  }}
                  placeholder="vd: http://103.14.22.1:8080 hoặc ip:port:user:pass"
                  className="bg-background border-input text-foreground text-xs font-mono placeholder:text-muted-foreground flex-1"
                />
              </div>

              {/* Proxy Test Feedback Badge */}
              {chromeProxyResult && (
                <div
                  className={`mt-1.5 flex items-center gap-1.5 p-1.5 rounded text-[11px] font-mono ${
                    chromeProxyResult.success
                      ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20"
                      : "bg-rose-500/10 text-rose-700 dark:text-rose-400 border border-rose-500/20"
                  }`}
                >
                  {chromeProxyResult.success ? (
                    <CheckCircle2 className="size-3 shrink-0 text-emerald-600 dark:text-emerald-400" />
                  ) : (
                    <XCircle className="size-3 shrink-0 text-destructive" />
                  )}
                  <span>
                    {chromeProxyResult.success
                      ? `Proxy hợp lệ! Độ trễ: ${chromeProxyResult.latency}ms${
                          chromeProxyResult.ip ? ` | IP: ${chromeProxyResult.ip}` : ""
                        }`
                      : chromeProxyResult.message}
                  </span>
                </div>
              )}

              <p className="text-[11px] text-muted-foreground mt-1">
                Chrome sẽ mở qua Proxy này để đăng nhập, hạn chế bị Google checkpoint vị trí.
              </p>
            </div>

            <div className="flex items-center justify-end gap-2 pt-3 border-t border-border">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={onClose}
                className="text-xs"
              >
                Hủy
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => {
                  onClose();
                  onStartChromeLogin(chromeProxy.trim());
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
              <label className="text-xs font-semibold text-foreground block mb-1">
                Chuỗi Cookie Google <span className="text-destructive">*</span>
              </label>
              <textarea
                value={cookies}
                onChange={(e) => setCookies(e.target.value)}
                placeholder="Dán chuỗi cookie (chứa __Secure-1PSID=...; SID=...; __Secure-1PSIDTS=...)"
                rows={3}
                required
                className="w-full rounded-lg bg-background border border-input p-2.5 text-xs font-mono text-foreground placeholder:text-muted-foreground focus:outline-hidden focus:border-ring focus:ring-1 focus:ring-ring/50 transition-all resize-y"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs font-medium text-foreground block mb-1">
                  Email Google <span className="text-destructive">*</span>:
                </label>
                <Input
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="vd: myaccount@gmail.com"
                  required
                  className="bg-background border-input text-foreground text-xs font-mono placeholder:text-muted-foreground"
                />
              </div>

              <div>
                <label className="text-xs font-medium text-foreground block mb-1">
                  Token SNlM0e (tùy chọn):
                </label>
                <Input
                  value={snlm0e}
                  onChange={(e) => setSnlm0e(e.target.value)}
                  placeholder="CSRF Token SNlM0e"
                  className="bg-background border-input text-foreground text-xs font-mono placeholder:text-muted-foreground"
                />
              </div>
            </div>

            <div className="space-y-3">
              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="text-xs font-medium text-foreground">
                    Proxy riêng (tùy chọn):
                  </label>
                  {proxy.trim() && (
                    <button
                      type="button"
                      onClick={handleTestManualProxy}
                      disabled={manualProxyTesting}
                      className="text-[11px] text-primary hover:underline font-medium cursor-pointer flex items-center gap-1"
                    >
                      {manualProxyTesting && <Loader2 className="size-2.5 animate-spin" />}
                      <span>Kiểm tra</span>
                    </button>
                  )}
                </div>
                <Input
                  value={proxy}
                  onChange={(e) => {
                    setProxy(e.target.value);
                    setManualProxyResult(null);
                  }}
                  placeholder="http://103.1.2.3:8080"
                  className="bg-background border-input text-foreground text-xs font-mono placeholder:text-muted-foreground"
                />
                {manualProxyResult && (
                  <div
                    className={`mt-1 flex items-center gap-1 p-1 rounded text-[10px] font-mono ${
                      manualProxyResult.success
                        ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20"
                        : "bg-rose-500/10 text-rose-700 dark:text-rose-400 border border-rose-500/20"
                    }`}
                  >
                    {manualProxyResult.success ? (
                      <CheckCircle2 className="size-2.5 shrink-0 text-emerald-600 dark:text-emerald-400" />
                    ) : (
                      <XCircle className="size-2.5 shrink-0 text-destructive" />
                    )}
                    <span>
                      {manualProxyResult.success
                        ? `Tốt (${manualProxyResult.latency}ms)`
                        : "Lỗi kết nối"}
                    </span>
                  </div>
                )}
              </div>

            </div>

            <div className="rounded-lg border border-border bg-muted/40 px-3 py-2 text-[11px] leading-relaxed text-muted-foreground">
              Hạn mức, credit và tier không nhập thủ công. Sau khi thêm tài khoản, hệ thống sẽ đọc trực tiếp từ Flow và Gemini rồi tự đồng bộ định kỳ.
            </div>

            <div className="flex items-center justify-end gap-2 pt-3 border-t border-border">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={onClose}
                disabled={isSubmitting}
                className="text-xs"
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
