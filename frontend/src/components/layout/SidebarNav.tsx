import { NavLink } from "react-router-dom";
import {
  LayoutDashboard,
  Users,
  Route,
  Server,
  KeyRound,
  Gauge,
  ScrollText,
  Settings,
} from "lucide-react";
import { cn } from "@/lib/utils";

const navItems = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { to: "/accounts", label: "Tài khoản Google", icon: Users },
  { to: "/api-keys", label: "API Keys", icon: KeyRound },
  { to: "/routes", label: "Routes", icon: Route },
  { to: "/upstreams", label: "Upstreams", icon: Server },
  { to: "/rate-limits", label: "Rate Limits", icon: Gauge },
  { to: "/logs", label: "Logs", icon: ScrollText },
  { to: "/settings", label: "Settings", icon: Settings },
];

export function SidebarNav() {
  return (
    <ul className="flex flex-col gap-1 px-3">
      {navItems.map((item) => (
        <li key={item.to}>
          <NavLink
            to={item.to}
            className={({ isActive }) =>
              cn(
                "group relative flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-all duration-150 select-none",
                isActive
                  ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold shadow-2xs"
                  : "text-sidebar-foreground/75 hover:bg-sidebar-accent/60 hover:text-sidebar-foreground"
              )
            }
          >
            {({ isActive }) => (
              <>
                {/* Active indicator bar on the left edge */}
                {isActive && (
                  <span className="absolute left-0 top-1.5 bottom-1.5 w-1 rounded-r-full bg-primary" />
                )}
                <item.icon
                  className={cn(
                    "size-4 shrink-0 transition-transform duration-150 group-hover:scale-105",
                    isActive
                      ? "text-primary"
                      : "text-sidebar-foreground/70 group-hover:text-sidebar-foreground"
                  )}
                />
                <span>{item.label}</span>
              </>
            )}
          </NavLink>
        </li>
      ))}
    </ul>
  );
}
