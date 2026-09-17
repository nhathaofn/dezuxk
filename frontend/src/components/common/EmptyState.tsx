import React from "react";
import { Button } from "@/components/ui/button";

interface EmptyStateProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
  isLoading?: boolean;
}

export function EmptyState({
  icon,
  title,
  description,
  actionLabel,
  onAction,
  isLoading = false,
}: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-border/80 bg-card/40 p-12 text-center animate-in fade-in duration-200">
      <div className="flex size-14 items-center justify-center rounded-2xl bg-muted/60 text-muted-foreground shadow-2xs mb-4">
        {icon}
      </div>

      <h3 className="text-base font-semibold text-foreground">{title}</h3>
      <p className="mt-1.5 max-w-sm text-sm text-muted-foreground leading-relaxed">
        {description}
      </p>

      {actionLabel && onAction && (
        <div className="mt-6">
          <Button onClick={onAction} disabled={isLoading} className="gap-2 shadow-sm">
            {actionLabel}
          </Button>
        </div>
      )}
    </div>
  );
}
