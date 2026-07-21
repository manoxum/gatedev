import { Ban, History, ScrollText, Shield, Ticket, UserCog, Wifi } from "lucide-react";
import { TabsList, TabsTrigger } from "@/components/ui/tabs";

interface HotspotTabsListProps {
  connectedCount: number;
  blockedCount: number;
}

// HotspotTabsList e so a lista de abas da tela de hotspot (icones +
// contadores) - extraida de pages/Hotspot.tsx para manter aquele
// arquivo dentro do limite de ~200 linhas deste repo (ver CLAUDE.md).
// Continua dentro de <Tabs> no componente pai: TabsList/TabsTrigger
// (Radix) usam contexto React, que atravessa normalmente essa borda
// de componente.
export function HotspotTabsList({ connectedCount, blockedCount }: HotspotTabsListProps) {
  return (
    // 7 abas nao cabem numa linha no celular - 4 colunas x 2 linhas
    // abaixo de sm, linha unica automatica no desktop.
    <TabsList className="grid h-auto w-full grid-cols-4 sm:inline-grid sm:w-auto sm:grid-cols-7">
      <TabsTrigger value="connected" className="gap-2">
        <Wifi className="h-4 w-4" />
        Conectados
        <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground">
          {connectedCount}
        </span>
      </TabsTrigger>
      <TabsTrigger value="blocked" className="gap-2">
        <Ban className="h-4 w-4" />
        Bloqueados
        <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground">
          {blockedCount}
        </span>
      </TabsTrigger>
      <TabsTrigger value="known" className="gap-2">
        <History className="h-4 w-4" />
        Todos os dispositivos
      </TabsTrigger>
      <TabsTrigger value="profiles">
        <UserCog className="h-4 w-4" />
        Perfis
      </TabsTrigger>
      <TabsTrigger value="isolation">
        <Shield className="h-4 w-4" />
        Isolamento
      </TabsTrigger>
      <TabsTrigger value="vouchers">
        <Ticket className="h-4 w-4" />
        Vouchers
      </TabsTrigger>
      <TabsTrigger value="logs">
        <ScrollText className="h-4 w-4" />
        Logs
      </TabsTrigger>
    </TabsList>
  );
}
