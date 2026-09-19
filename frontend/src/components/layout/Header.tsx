import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { getGatewayStatus, toggleGateway, type GatewayStatus } from "@/lib/api/gateway";
import { toast } from "@/lib/toast";
import { Loader2 } from "lucide-react";

const pageTitles: Record<string, string> = {
  "/dashboard": "Dashboard",
  "/accounts": "Tài khoản Google · Flow + Gemini",
  "/routes": "Routes",
  "/upstreams": "Upstreams",
  "/api-keys": "API Keys",
  "/rate-limits": "Rate Limits",
  "/logs": "Logs",
  "/settings": "Settings",
};

export function Header() {
  const location = useLocation();
  const title = pageTitles[location.pathname] ?? "Gateway Manager";
  const [status, setStatus] = useState<GatewayStatus>({
    isRunning: false,
    ip: "",
    port: 8080,
  });
  const [isToggling, setIsToggling] = useState(false);

  useEffect(() => {
    let mounted = true;
    getGatewayStatus().then((res) => {
      if (mounted) setStatus(res);
    });

    const interval = setInterval(() => {
      getGatewayStatus().then((res) => {
        if (mounted) setStatus(res);
      });
    }, 4000);

    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, []);

  const handleToggle = async () => {
    setIsToggling(true);
    try {
      const res = await toggleGateway();
      setStatus(res);
      if (res.isRunning) {
        toast.success("Đã bật máy chủ Gateway", `${res.ip}:${res.port}`);
      } else {
        toast.info("Đã tắt máy chủ Gateway");
      }
    } catch (err: any) {
      toast.error("Lỗi thay đổi trạng thái Gateway", err?.message);
    } finally {
      setIsToggling(false);
    }
  };

  return (
    <header className="flex h-12 items-center justify-between border-b border-border bg-background px-6 select-none">
      <h1 className="text-sm font-semibold text-foreground tracking-tight">
        {title}
      </h1>

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={handleToggle}
          disabled={isToggling}
          title={status.isRunning ? "Nhấn để tắt Gateway server" : "Nhấn để bật Gateway server"}
          className="group flex items-center gap-2 rounded-full border border-border/80 bg-muted/40 px-3 py-1 text-xs font-medium transition-all hover:bg-muted"
        >
          {isToggling ? (
            <Loader2 className="size-3.5 animate-spin text-muted-foreground" />
          ) : status.isRunning ? (
            <>
              <span className="relative flex size-2">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex size-2 rounded-full bg-emerald-500"></span>
              </span>
              <span className="font-mono text-emerald-600 dark:text-emerald-400 font-semibold">
                {status.ip}:{status.port}
              </span>
            </>
          ) : (
            <>
              <span className="size-2 rounded-full bg-muted-foreground/40"></span>
              <span className="text-muted-foreground">Chưa bật</span>
            </>
          )}
        </button>
      </div>
    </header>
  );
}
