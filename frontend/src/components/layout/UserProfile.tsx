import { useAuth } from "@/context/AuthContext";
import { LogOut, User as UserIcon } from "lucide-react";

export function UserProfile() {
  const { user, logout } = useAuth();

  if (!user) return null;

  return (
    <div className="border-t border-border p-3">
      <div className="flex items-center justify-between gap-2 rounded-lg bg-sidebar-accent/50 p-2">
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
            <UserIcon className="size-4" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-xs font-medium text-sidebar-foreground">
              {user.username}
            </p>
          </div>
        </div>

        <button
          type="button"
          onClick={() => logout()}
          title="Đăng xuất"
          className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
        >
          <LogOut className="size-3.5" />
        </button>
      </div>
    </div>
  );
}
