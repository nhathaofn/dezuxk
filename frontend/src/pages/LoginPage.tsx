import React, { useEffect, useState } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { useAuth } from "@/context/AuthContext";
import { Button } from "@/components/ui/button";
import { PasswordInput } from "@/components/ui/password-input";
import { Alert } from "@/components/ui/alert";
import { Card, CardContent } from "@/components/ui/card";
import { BrandLogo } from "@/components/common/BrandLogo";
import { Loader2 } from "lucide-react";
import { isAuthSetupRequired } from "@/lib/api/auth";
import { MIN_PASSWORD_LENGTH } from "@/lib/constants";

export function LoginPage() {
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [setupRequired, setSetupRequired] = useState(false);
  const [setupLoading, setSetupLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const { login, register } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const from = (location.state as any)?.from?.pathname || "/dashboard";

  useEffect(() => {
    let mounted = true;
    isAuthSetupRequired()
      .then((required) => {
        if (mounted) setSetupRequired(required);
      })
      .catch(() => {
        if (mounted) setSetupRequired(false);
      })
      .finally(() => {
        if (mounted) setSetupLoading(false);
      });
    return () => {
      mounted = false;
    };
  }, []);

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setError(null);

    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`Mật khẩu phải có ít nhất ${MIN_PASSWORD_LENGTH} ký tự.`);
      return;
    }
    if (setupRequired && password !== confirmPassword) {
      setError("Mật khẩu xác nhận không khớp.");
      return;
    }

    setIsSubmitting(true);
    try {
      if (setupRequired) {
        await register("admin", password);
      } else {
        await login("admin", password);
      }
      navigate(from, { replace: true });
    } catch (err: any) {
      const message =
        err?.message ||
        (typeof err === "string"
          ? err
          : "Mật khẩu không chính xác. Vui lòng thử lại.");
      setError(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-screen w-full items-center justify-center bg-background px-4 py-8">
      <div className="w-full max-w-sm space-y-6">
        {/* Brand Logo Header */}
        <BrandLogo size="lg" orientation="vertical" />

        {/* Card Form */}
        <Card className="shadow-md">
          <CardContent className="p-6">
            {setupRequired && !setupLoading && (
              <p className="mb-4 text-xs leading-relaxed text-muted-foreground">
                Đây là lần chạy đầu tiên. Hãy tạo mật khẩu quản trị viên tối thiểu 5 ký tự.
              </p>
            )}
            {error && (
              <Alert variant="destructive" className="mb-4">
                {error}
              </Alert>
            )}

            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <label
                  htmlFor="current-password"
                  className="block text-xs font-medium text-foreground"
                >
                  {setupRequired ? "Tạo mật khẩu quản trị" : "Mật khẩu"}
                </label>
                <PasswordInput
                  id="current-password"
                  name="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Nhập mật khẩu..."
                  autoFocus
                  required
                  disabled={isSubmitting || setupLoading}
                />
              </div>

              {setupRequired && (
                <div className="space-y-1.5">
                  <label
                    htmlFor="confirm-password"
                    className="block text-xs font-medium text-foreground"
                  >
                    Xác nhận mật khẩu
                  </label>
                  <PasswordInput
                    id="confirm-password"
                    name="confirmPassword"
                    autoComplete="new-password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    placeholder="Nhập lại mật khẩu..."
                    required
                    disabled={isSubmitting}
                  />
                </div>
              )}

              <Button
                type="submit"
                className="w-full justify-center text-sm font-medium"
                disabled={isSubmitting}
              >
                {isSubmitting ? (
                  <>
                    <Loader2 className="mr-2 size-4 animate-spin" />
                    {setupRequired ? "Đang tạo tài khoản..." : "Đang xác thực..."}
                  </>
                ) : (
                  setupRequired ? "Tạo tài khoản quản trị" : "Đăng nhập"
                )}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
