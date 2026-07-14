import { useEffect, useRef, useState } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export interface InterfaceQuickSwitchOption {
  value: string;
  label: string;
}

interface HotspotInterfaceQuickSwitchProps {
  icon: LucideIcon;
  label: string;
  value: string;
  // Texto mostrado no card fechado quando difere do "value" cru (ex.:
  // "auto (eth0)" combinando o valor configurado com o resolvido pelo
  // worker) - o "value" em si continua sendo o que value casa contra
  // "options" para destacar a selecao atual e decidir se mudou.
  displayValue?: string;
  options: InterfaceQuickSwitchOption[];
  onChange: (value: string) => void;
  disabled?: boolean;
}

// Card compacto (mesmo visual de ConfigItem em HotspotSummaryCard) que
// vira um dropdown ao clicar, para trocar a interface Wi-Fi/de internet
// direto do resumo, sem abrir o dialog inteiro de "Alterar configuração".
// Usa o mesmo fluxo de salvar+aplicar (onChange chama saveAndApply) - a
// escolha do usuário aqui já é a confirmação, igual ao botão "Salvar e
// aplicar" do dialog completo.
export function HotspotInterfaceQuickSwitch({
  icon: Icon,
  label,
  value,
  displayValue,
  options,
  onChange,
  disabled,
}: HotspotInterfaceQuickSwitchProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);

  const selectedLabel = displayValue ?? options.find((option) => option.value === value)?.label ?? value;

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        disabled={disabled}
        className="flex w-full appearance-none items-center gap-2 rounded-lg border border-border/60 bg-muted/30 px-2 py-1.5 text-left font-sans text-foreground transition-colors hover:bg-muted/60 disabled:cursor-not-allowed disabled:opacity-60"
      >
        <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <Icon className="h-3.5 w-3.5" />
        </div>
        <div className="min-w-0">
          <p className="text-[10px] leading-tight text-muted-foreground">{label}</p>
          <p className="truncate text-xs font-semibold leading-tight">{selectedLabel || "—"}</p>
        </div>
      </button>
      {open && (
        <div className="absolute left-0 top-full z-10 mt-1 w-56 max-h-64 overflow-auto rounded-md border border-border bg-popover p-1 shadow-md">
          {options.map((option) => (
            <button
              key={option.value}
              type="button"
              onClick={() => {
                setOpen(false);
                if (option.value !== value) onChange(option.value);
              }}
              className={cn(
                "block w-full appearance-none rounded-sm px-2 py-1.5 text-left text-xs text-foreground hover:bg-accent hover:text-accent-foreground",
                option.value === value && "bg-accent/50 font-medium",
              )}
            >
              {option.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
