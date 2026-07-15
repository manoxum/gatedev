import { Globe, ScrollText, Settings } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { LogsPanel } from "@/components/LogsPanel";
import { DiscoverMeshConfigCard } from "@/components/DiscoverMeshConfigCard";
import { DnsRecordsCard } from "@/components/dns/DnsRecordsCard";
import { DnsTestResolutionCard } from "@/components/dns/DnsTestResolutionCard";
import { DnsLocalTldsCard } from "@/components/dns/DnsLocalTldsCard";
import { useDnsQueries } from "@/components/dns/useDnsQueries";
import { useDnsMutations } from "@/components/dns/useDnsMutations";
import { usePageHeader } from "@/hooks/usePageHeader";
import { useUrlTab } from "@/hooks/useUrlTab";

export function DnsPage() {
  usePageHeader({ title: "DNS local (split-horizon)", description: "TLDs locais, malha de descoberta e testes de resolução." });

  const { config, records } = useDnsQueries();
  const mutations = useDnsMutations();
  const [tab, setTab] = useUrlTab("records");

  return (
    <Tabs value={tab} onValueChange={setTab} className="space-y-4">
      <TabsList className="grid h-auto w-full grid-cols-3 sm:inline-grid sm:w-auto">
        <TabsTrigger value="records" className="gap-2">
          <Globe className="h-4 w-4" />
          Resolvidos
          <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground">
            {records.data?.length ?? 0}
          </span>
        </TabsTrigger>
        <TabsTrigger value="config" className="gap-2">
          <Settings className="h-4 w-4" />
          Configuração
        </TabsTrigger>
        <TabsTrigger value="logs" className="gap-2">
          <ScrollText className="h-4 w-4" />
          Logs
        </TabsTrigger>
      </TabsList>

      <TabsContent value="records" className="mt-0 space-y-4">
        <DnsRecordsCard records={records.data ?? []} mutations={mutations} />
        <DnsTestResolutionCard mutations={mutations} />
      </TabsContent>

      <TabsContent value="config" className="mt-0 space-y-4">
        <DnsLocalTldsCard config={config.data} mutations={mutations} />
        <DiscoverMeshConfigCard />
      </TabsContent>

      <TabsContent value="logs" className="mt-0">
        <LogsPanel title="Logs do DNS" path="/dns/logs" />
      </TabsContent>
    </Tabs>
  );
}
