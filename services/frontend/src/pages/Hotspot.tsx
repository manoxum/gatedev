import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { HotspotConfigForm } from "@/components/hotspot/HotspotConfigForm";
import { HotspotBlocklistCard } from "@/components/hotspot/HotspotBlocklistCard";
import { HotspotClientsCard } from "@/components/hotspot/HotspotClientsCard";
import { HotspotSummaryCard } from "@/components/hotspot/HotspotSummaryCard";
import { GlobalLimitsCard } from "@/components/hotspot/GlobalLimitsCard";
import { configSchema, type ConfigForm } from "@/components/hotspot/hotspot-schema";
import { useHotspotQueries } from "@/components/hotspot/useHotspotQueries";
import { useHotspotMutations } from "@/components/hotspot/useHotspotMutations";
import { LogsPanel } from "@/components/LogsPanel";
import { usePageHeader } from "@/hooks/usePageHeader";

export function HotspotPage() {
  const [showPassword, setShowPassword] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);
  const [confirmRecoverOpen, setConfirmRecoverOpen] = useState(false);

  const { status, config, interfaces, clients, blocklist } = useHotspotQueries();
  const { save, apply, start, stop, recoverWifi, identify, block, unblock } = useHotspotMutations({
    onSaveSuccess: () => setConfigOpen(false),
    onRecoverSuccess: () => setConfirmRecoverOpen(false),
  });

  const { register, handleSubmit, reset, formState: { errors, isDirty } } = useForm<ConfigForm>({
    resolver: zodResolver(configSchema),
  });

  useEffect(() => {
    if (config.data) {
      reset({
        WIFI_SSID: config.data.WIFI_SSID ?? "",
        WIFI_PASSWORD: config.data.WIFI_PASSWORD ?? "",
        WIFI_INTERFACE: config.data.WIFI_INTERFACE ?? "",
        INTERNET_INTERFACE: config.data.INTERNET_INTERFACE ?? "",
        WIFI_COUNTRY: config.data.WIFI_COUNTRY ?? "ST",
        WIFI_CHANNEL: config.data.WIFI_CHANNEL ?? "auto",
        WIFI_FREQ_BAND: config.data.WIFI_FREQ_BAND ?? "auto",
      });
    }
  }, [config.data, reset]);

  const wifiInterfaces = interfaces.data?.filter((i) => i.type === "wifi") ?? [];
  const networkInterfaces = interfaces.data ?? [];
  const connectedCount = clients.data?.length ?? 0;
  const blockedCount = blocklist.data?.length ?? 0;

  usePageHeader({
    title: "Hotspot Wi-Fi",
    description: status.data?.running
      ? `Rodando · canal ${status.data.channel ?? "?"} · banda ${status.data.band ?? "?"}GHz`
      : "Parado",
  });

  return (
    <div className="space-y-6">
      <HotspotSummaryCard
        config={config.data ?? {}}
        running={!!status.data?.running}
        startPending={start.isPending}
        stopPending={stop.isPending}
        recoverPending={recoverWifi.isPending}
        onStart={() => start.mutate()}
        onStop={() => stop.mutate()}
        onRecover={() => {
          if (status.data?.running) {
            setConfirmRecoverOpen(true);
            return;
          }
          recoverWifi.mutate();
        }}
        onEdit={() => setConfigOpen(true)}
      />

      <Tabs defaultValue="connected" className="space-y-4">
        <TabsList className="grid h-auto w-full grid-cols-4 sm:inline-grid sm:w-auto">
          <TabsTrigger value="connected" className="gap-2">
            Conectados
            <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground">
              {connectedCount}
            </span>
          </TabsTrigger>
          <TabsTrigger value="blocked" className="gap-2">
            Bloqueados
            <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground">
              {blockedCount}
            </span>
          </TabsTrigger>
          <TabsTrigger value="limits">Limites</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="connected" className="mt-0">
          <HotspotClientsCard
            clients={clients.data ?? []}
            running={!!status.data?.running}
            identifyPendingMac={identify.isPending ? identify.variables : undefined}
            blockPendingMac={block.isPending ? block.variables : undefined}
            unblockPendingMac={unblock.isPending ? unblock.variables : undefined}
            onIdentify={(mac) => identify.mutate(mac)}
            onBlock={(mac) => block.mutate(mac)}
            onUnblock={(mac) => unblock.mutate(mac)}
          />
        </TabsContent>

        <TabsContent value="blocked" className="mt-0">
          <HotspotBlocklistCard
            devices={blocklist.data ?? []}
            unblockPendingMac={unblock.isPending ? unblock.variables : undefined}
            onUnblock={(mac) => unblock.mutate(mac)}
          />
        </TabsContent>

        <TabsContent value="limits" className="mt-0">
          <GlobalLimitsCard />
        </TabsContent>

        <TabsContent value="logs" className="mt-0">
          <LogsPanel title="Logs do hotspot" path="/hotspot/logs" />
        </TabsContent>
      </Tabs>

      <Dialog open={configOpen} onOpenChange={setConfigOpen}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Configuração do hotspot</DialogTitle>
            <DialogDescription>
              Salvar grava no .env; "Aplicar" recria o hotspot para assumir os novos valores (derruba a conexão por
              alguns segundos).
            </DialogDescription>
          </DialogHeader>
          <HotspotConfigForm
            register={register}
            errors={errors}
            handleSubmit={handleSubmit}
            onSave={(data) => save.mutate(data)}
            onApply={() => apply.mutate()}
            isDirty={isDirty}
            savePending={save.isPending}
            applyPending={apply.isPending}
            showPassword={showPassword}
            onToggleShowPassword={() => setShowPassword((v) => !v)}
            wifiInterfaces={wifiInterfaces}
            networkInterfaces={networkInterfaces}
          />
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmRecoverOpen}
        onOpenChange={setConfirmRecoverOpen}
        title="Recuperar adaptador Wi-Fi"
        description="O hotspot está ligado e vai ser parado agora para recuperar o adaptador. Continuar?"
        confirmLabel="Recuperar"
        variant="destructive"
        pending={recoverWifi.isPending}
        onConfirm={() => recoverWifi.mutate()}
      />
    </div>
  );
}
