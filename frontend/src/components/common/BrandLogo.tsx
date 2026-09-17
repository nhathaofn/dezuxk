import React from "react";
import { ShieldCheck } from "lucide-react";
import { cn } from "@/lib/utils";

interface BrandLogoProps {
  size?: "sm" | "md" | "lg";
  orientation?: "horizontal" | "vertical";
  className?: string;
}

export function BrandLogo({
  size = "md",
  orientation = "horizontal",
  className,
}: BrandLogoProps) {
  const isVertical = orientation === "vertical";

  const iconSizes = {
    sm: "size-6 rounded-md",
    md: "size-8 rounded-lg",
    lg: "size-14 rounded-2xl",
  };

  const svgSizes = {
    sm: "size-3.5",
    md: "size-4.5",
    lg: "size-7",
  };

  const textSizes = {
    sm: "text-sm font-semibold",
    md: "text-base font-semibold",
    lg: "text-2xl font-bold tracking-tight",
  };

  return (
    <div
      className={cn(
        "flex items-center select-none",
        isVertical ? "flex-col gap-3 text-center" : "flex-row gap-2.5 text-left",
        className
      )}
    >
      <div
        className={cn(
          "flex items-center justify-center bg-primary text-primary-foreground shadow-sm shrink-0",
          iconSizes[size]
        )}
      >
        <ShieldCheck className={svgSizes[size]} />
      </div>

      <span
        className={cn("text-foreground tracking-tight", textSizes[size])}
      >
        Gateway Manager
      </span>
    </div>
  );
}
