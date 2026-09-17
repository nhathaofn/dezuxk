import * as React from "react";
import { useState } from "react";
import { Eye, EyeOff, Lock } from "lucide-react";
import { Input } from "./input";
import { cn } from "@/lib/utils";

export interface PasswordInputProps
  extends Omit<React.ComponentProps<typeof Input>, "type"> {
  showLeadingIcon?: boolean;
}

const PasswordInput = React.forwardRef<HTMLInputElement, PasswordInputProps>(
  ({ className, showLeadingIcon = true, disabled, ...props }, ref) => {
    const [showPassword, setShowPassword] = useState(false);

    return (
      <div className="relative flex items-center">
        {showLeadingIcon && (
          <div className="pointer-events-none absolute left-3 flex items-center text-foreground/60">
            <Lock className="size-4" />
          </div>
        )}

        <Input
          type={showPassword ? "text" : "password"}
          className={cn(
            showLeadingIcon && "pl-9",
            "pr-9",
            className
          )}
          ref={ref}
          disabled={disabled}
          {...props}
        />

        <button
          type="button"
          onClick={() => setShowPassword((prev) => !prev)}
          disabled={disabled}
          tabIndex={-1}
          aria-label={showPassword ? "Ẩn mật khẩu" : "Hiện mật khẩu"}
          className="absolute right-0 flex h-full items-center px-3 text-foreground/60 transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
        >
          {showPassword ? (
            <EyeOff className="size-4" />
          ) : (
            <Eye className="size-4" />
          )}
        </button>
      </div>
    );
  }
);

PasswordInput.displayName = "PasswordInput";

export { PasswordInput };
