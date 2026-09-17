import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { AlertCircle, CheckCircle2, Info } from "lucide-react";

const alertVariants = cva(
  "relative w-full rounded-lg border px-3.5 py-3 text-xs [&>svg]:absolute [&>svg]:left-3.5 [&>svg]:top-3 [&>svg]:size-4 [&>svg~*]:pl-6 flex items-start gap-2.5",
  {
    variants: {
      variant: {
        default: "bg-background text-foreground border-border [&>svg]:text-foreground",
        destructive:
          "border-destructive/30 bg-destructive/10 text-destructive dark:border-destructive/40 [&>svg]:text-destructive",
        success:
          "border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-400 [&>svg]:text-green-600 dark:[&>svg]:text-green-400",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

interface AlertProps
  extends React.ComponentProps<"div">,
    VariantProps<typeof alertVariants> {
  icon?: React.ReactNode;
}

function Alert({ className, variant = "default", icon, children, ...props }: AlertProps) {
  const defaultIcon =
    variant === "destructive" ? (
      <AlertCircle className="size-4 shrink-0" />
    ) : variant === "success" ? (
      <CheckCircle2 className="size-4 shrink-0" />
    ) : (
      <Info className="size-4 shrink-0" />
    );

  return (
    <div
      role="alert"
      className={cn(alertVariants({ variant }), className)}
      {...props}
    >
      {icon ?? defaultIcon}
      <div className="flex-1 leading-relaxed">{children}</div>
    </div>
  );
}

export { Alert, alertVariants };
