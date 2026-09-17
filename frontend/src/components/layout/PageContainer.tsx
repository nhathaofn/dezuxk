import React from "react";
import { cn } from "@/lib/utils";

interface PageContainerProps {
  title: string;
  children?: React.ReactNode;
  className?: string;
}

export function PageContainer({ title, children, className }: PageContainerProps) {
  return (
    <div
      className={cn(
        "flex w-full flex-col items-center justify-center text-center p-8 select-none",
        className
      )}
    >
      <h2 className="text-xl font-semibold tracking-tight text-foreground sm:text-2xl">
        {title}
      </h2>
      {children}
    </div>
  );
}

