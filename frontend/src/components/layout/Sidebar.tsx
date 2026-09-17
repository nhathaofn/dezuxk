import { SidebarNav } from "./SidebarNav";
import { BrandLogo } from "@/components/common/BrandLogo";
import { UserProfile } from "./UserProfile";

export function Sidebar() {
  return (
    <aside className="flex h-full w-56 flex-col border-r border-border bg-sidebar select-none">
      {/* Brand Header */}
      <div className="flex h-12 items-center border-b border-border px-4">
        <BrandLogo size="sm" />
      </div>

      {/* Navigation items */}
      <nav className="flex-1 overflow-auto py-2">
        <SidebarNav />
      </nav>

      {/* User profile & Logout footer */}
      <UserProfile />
    </aside>
  );
}
