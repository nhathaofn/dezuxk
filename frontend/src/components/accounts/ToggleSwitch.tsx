import React from "react";

interface ToggleSwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  size?: "sm" | "md";
  className?: string;
}

export function ToggleSwitch({
  checked,
  onChange,
  disabled = false,
  size = "md",
  className = "",
}: ToggleSwitchProps) {
  const isSm = size === "sm";

  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => !disabled && onChange(!checked)}
      className={`relative inline-flex shrink-0 cursor-pointer items-center rounded-full transition-colors duration-200 ease-in-out focus:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 ${
        isSm ? "h-5 w-9" : "h-6 w-11"
      } ${
        checked
          ? "bg-emerald-500 shadow-2xs shadow-emerald-500/30"
          : "bg-muted-foreground/30 hover:bg-muted-foreground/40"
      } ${className}`}
    >
      <span
        className={`pointer-events-none inline-block transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out ${
          isSm
            ? `h-3.5 w-3.5 ${checked ? "translate-x-4.5" : "translate-x-1"}`
            : `h-5 w-5 ${checked ? "translate-x-5.5" : "translate-x-0.5"}`
        }`}
      />
    </button>
  );
}
