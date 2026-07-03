import * as React from "react";
import { cn } from "@/lib/utils";

// Barra de progresso simples via div (em vez de @radix-ui/react-progress) -
// mesmo raciocínio de select-native.tsx: evita mais uma dependência para
// algo que não precisa de nenhuma interatividade.
export interface ProgressProps extends React.HTMLAttributes<HTMLDivElement> {
  value: number;
}

export function Progress({ value, className, ...props }: ProgressProps) {
  const clamped = Math.min(100, Math.max(0, value));
  return (
    <div className={cn("h-2 w-full overflow-hidden rounded-full bg-muted", className)} {...props}>
      <div
        className={cn("h-full rounded-full transition-all", clamped >= 100 ? "bg-destructive" : "bg-primary")}
        style={{ width: `${clamped}%` }}
      />
    </div>
  );
}
