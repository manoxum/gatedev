import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Ban, History, ScrollText, Sliders, Ticket, UserCog, Wifi } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { HotspotConfigForm } from "@/components/hotspot/HotspotConfigForm";
import { HotspotBlocklistCard } from "@/components/hotspot/HotspotBlocklistCard";
import { HotspotClientsCard } from "@/components/hotspot/HotspotClientsCard";
import { HotspotKnownDevicesCard } from "@/components/hotspot/HotspotKnownDevicesCard";
import { HotspotSummaryCard } from "@/components/hotspot/HotspotSummaryCard";
import { GlobalLimitsCard } from "@/components/hotspot/GlobalLimitsCard";
import { HotspotProfilesCard } from "@/components/hotspot/HotspotProfilesCard";
import { HotspotVouchersCard } from "@/components/hotspot/HotspotVouchersCard";
import { configSchema, type ConfigForm } from "@/components/hotspot/hotspot-schema";
import { generateRandomWifiPassword } from "@/components/hotspot/generate-password";
import { useHotspotQueries } from "@/components/hotspot/useHotspotQueries";
import { useHotspotMutations } from "@/components/hotspot/useHotspotMutations";
import { LogsPanel } from "@/components/LogsPanel";
import { usePageHeader } from "@/hooks/usePageHeader";

export function HotspotPage() {
  const [showPassword, setShowPassword] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);
  const [confirmRecoverOpen, setConfirmRecoverOpen] = useState(false);
  const autoPromptedRef = useRef(false);

  const { status, config, interfaces, clients, blocklist, knownDevices } = useHotspotQueries();
  const { saveAndApply, start, stop, recoverWifi, block, unblock } = useHotspotMutations({
    onSaveSuccess: () => setConfigOpen(false),
    onRecoverSuccess: () => setConfirmRecoverOpen(false),
  });

  const { register, handleSubmit, reset, formState: { errors, isDirty } } = useForm<ConfigForm>({
    resolver: zodResolver(configSchema),
  });

  const wifiInterfaces = interfaces.data?.filter((i) => i.type === "wifi") ?? [];
  const networkInterfaces = interfaces.data ?? [];

  // Preenche o formulario assim que config+interfaces carregarem. Quando
  // ainda nao configurado (instalacao nova), sugere valores inteligentes
  // em vez de deixar em branco: interface Wi-Fi unica ja vem selecionada e
  // uma senha aleatoria ja vem pronta pro operador aceitar ou trocar. Se
  // SSID/interface ainda estiverem vazios, abre o dialogo de configuracao
  // automaticamente uma unica vez, em vez de depender do operador saber
  // clicar em "Alterar configuracao".
  useEffect(() => {
    if (!config.data || !interfaces.data) return;
    const needsSetup = !config.data.WIFI_SSID || !config.data.WIFI_INTERFACE;
    const suggestedInterface =
      config.data.WIFI_INTERFACE || (wifiInterfaces.length === 1 ? wifiInterfaces[0].name : "");
    reset({
      WIFI_SSID: config.data.WIFI_SSID ?? "",
      WIFI_PASSWORD: config.data.WIFI_PASSWORD || generateRandomWifiPassword(),
      WIFI_INTERFACE: suggestedInterface,
      INTERNET_INTERFACE: config.data.INTERNET_INTERFACE || "auto",
      WIFI_COUNTRY: config.data.WIFI_COUNTRY ?? "ST",
      WIFI_CHANNEL: config.data.WIFI_CHANNEL ?? "auto",
      WIFI_FREQ_BAND: config.data.WIFI_FREQ_BAND ?? "auto",
      WIFI_CHANNEL_CANDIDATES: config.data.WIFI_CHANNEL_CANDIDATES ?? "",
      HOTSPOT_GATEWAY: config.data.HOTSPOT_GATEWAY || "192.168.12.1",
      HOTSPOT_CIDR: config.data.HOTSPOT_CIDR || "192.168.12.0/24",
      HOTSPOT_DNS_FALLBACKS: config.data.HOTSPOT_DNS_FALLBACKS ?? "1.1.1.1,8.8.8.8",
      BINDNET_UPLINK_INTERFACE: config.data.BINDNET_UPLINK_INTERFACE || "bn-uplink",
      UPLINK_MONITOR_INTERVAL: config.data.UPLINK_MONITOR_INTERVAL || "10",
    });
    if (needsSetup && !autoPromptedRef.current) {
      autoPromptedRef.current = true;
      setConfigOpen(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [config.data, interfaces.data, reset]);

  const connectedCount = clients.data?.length ?? 0;
  const blockedCount = blocklist.data?.length ?? 0;
  const blockedMacs = new Set(blocklist.data?.map((device) => device.macAddress) ?? []);

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
        <TabsList className="grid h-auto w-full grid-cols-7 sm:inline-grid sm:w-auto">
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
          <TabsTrigger value="limits">
            <Sliders className="h-4 w-4" />
            Limites
          </TabsTrigger>
          <TabsTrigger value="profiles">
            <UserCog className="h-4 w-4" />
            Perfis
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

        <TabsContent value="connected" className="mt-0">
          <HotspotClientsCard
            clients={clients.data ?? []}
            running={!!status.data?.running}
            blockPendingMac={block.isPending ? block.variables.mac : undefined}
            unblockPendingMac={unblock.isPending ? unblock.variables : undefined}
            onBlock={(mac, mode) => block.mutate({ mac, mode })}
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

        <TabsContent value="known" className="mt-0">
          <HotspotKnownDevicesCard
            devices={knownDevices.data ?? []}
            blockedMacs={blockedMacs}
            blockPendingMac={block.isPending ? block.variables.mac : undefined}
            unblockPendingMac={unblock.isPending ? unblock.variables : undefined}
            onBlock={(mac, mode) => block.mutate({ mac, mode })}
            onUnblock={(mac) => unblock.mutate(mac)}
          />
        </TabsContent>

        <TabsContent value="limits" className="mt-0">
          <GlobalLimitsCard />
        </TabsContent>

        <TabsContent value="profiles" className="mt-0">
          <HotspotProfilesCard />
        </TabsContent>

        <TabsContent value="vouchers" className="mt-0">
          <HotspotVouchersCard />
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
              Salvar já recria o hotspot na hora para assumir os novos valores.
            </DialogDescription>
          </DialogHeader>
          <HotspotConfigForm
            register={register}
            errors={errors}
            handleSubmit={handleSubmit}
            onSave={(data) => saveAndApply.mutate(data)}
            isDirty={isDirty}
            savePending={saveAndApply.isPending}
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
