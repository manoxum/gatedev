import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { HotspotDialogs } from "@/components/hotspot/HotspotDialogs";
import { HotspotTabsList } from "@/components/hotspot/HotspotTabsList";
import { HotspotBlocklistCard } from "@/components/hotspot/HotspotBlocklistCard";
import { HotspotClientsCard } from "@/components/hotspot/HotspotClientsCard";
import { HotspotKnownDevicesCard } from "@/components/hotspot/HotspotKnownDevicesCard";
import { HotspotSummaryCard } from "@/components/hotspot/HotspotSummaryCard";
import { interfaceLabel } from "@/components/hotspot/HotspotInterfacesTab";
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
  const { saveAndApply, start, stop, recoverWifi, block, unblock, clearLogs } = useHotspotMutations({
    onSaveSuccess: () => setConfigOpen(false),
    onRecoverSuccess: () => setConfirmRecoverOpen(false),
  });

  const { register, handleSubmit, reset, formState: { errors, isDirty } } = useForm<ConfigForm>({
    resolver: zodResolver(configSchema),
  });

  const wifiInterfaces = interfaces.data?.filter((i) => i.type === "wifi") ?? [];
  const networkInterfaces = interfaces.data ?? [];
  const wifiInterfaceOptions = wifiInterfaces.map((i) => ({ value: i.name, label: `${i.name} (${i.state})` }));
  const internetInterfaceOptions = [
    { value: "auto", label: "Automática (melhor disponível)" },
    ...networkInterfaces.map((i) => ({ value: i.name, label: interfaceLabel(i) })),
  ];

  // Troca rapida de interface pelo card de resumo (sem abrir o dialog
  // inteiro de "Alterar configuracao") - reusa o mesmo salvar+aplicar
  // do formulario completo; a escolha do usuario no dropdown ja e a
  // confirmacao, igual clicar em "Salvar e aplicar" no dialog.
  const handleQuickSwitchInterface = (field: "WIFI_INTERFACE" | "INTERNET_INTERFACE", value: string) => {
    if (!config.data) return;
    saveAndApply.mutate({ ...config.data, [field]: value } as ConfigForm);
  };

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
        currentBand={status.data?.band}
        currentChannel={status.data?.channel}
        currentInternetInterface={status.data?.internetInterface}
        wifiInterfaceOptions={wifiInterfaceOptions}
        internetInterfaceOptions={internetInterfaceOptions}
        onQuickSwitchInterface={handleQuickSwitchInterface}
        quickSwitchPending={saveAndApply.isPending}
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
        <HotspotTabsList connectedCount={connectedCount} blockedCount={blockedCount} />

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
          <LogsPanel title="Logs do hotspot" path="/hotspot/logs" onClear={() => clearLogs.mutateAsync()} />
        </TabsContent>
      </Tabs>

      <HotspotDialogs
        configOpen={configOpen}
        onConfigOpenChange={setConfigOpen}
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
        confirmRecoverOpen={confirmRecoverOpen}
        onConfirmRecoverOpenChange={setConfirmRecoverOpen}
        recoverPending={recoverWifi.isPending}
        onConfirmRecover={() => recoverWifi.mutate()}
      />
    </div>
  );
}
