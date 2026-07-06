import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardFooter } from "@/components/ui/card";
import { HotspotConfigForm } from "@/components/hotspot/HotspotConfigForm";
import { configSchema, type ConfigForm } from "@/components/hotspot/hotspot-schema";
import { generateRandomWifiPassword } from "@/components/hotspot/generate-password";
import { useHotspotQueries } from "@/components/hotspot/useHotspotQueries";

interface SetupHotspotStepProps {
  initialData?: ConfigForm;
  onNext: (data: ConfigForm) => void;
  onSkip: () => void;
}

// Só coleta os valores do formulário - nada é salvo/aplicado aqui, isso
// só acontece no último passo (SetupStatusStep), de uma vez só.
export function SetupHotspotStep({ initialData, onNext, onSkip }: SetupHotspotStepProps) {
  const [showPassword, setShowPassword] = useState(false);
  const { config, interfaces } = useHotspotQueries();

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isDirty },
  } = useForm<ConfigForm>({ resolver: zodResolver(configSchema) });

  const wifiInterfaces = interfaces.data?.filter((i) => i.type === "wifi") ?? [];
  const networkInterfaces = interfaces.data ?? [];

  useEffect(() => {
    if (initialData) {
      reset(initialData);
      return;
    }
    if (!config.data || !interfaces.data) return;
    const suggestedInterface =
      config.data.WIFI_INTERFACE || (wifiInterfaces.length === 1 ? wifiInterfaces[0].name : "");
    reset({
      WIFI_SSID: config.data.WIFI_SSID || "Bindnet",
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialData, config.data, interfaces.data, reset]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Hotspot Wi-Fi</CardTitle>
        <CardDescription>
          Defina o SSID, a senha e as interfaces do hotspot. A configuração só é salva e aplicada ao final do
          assistente.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <HotspotConfigForm
          register={register}
          errors={errors}
          handleSubmit={handleSubmit}
          onSave={() => {}}
          isDirty={isDirty}
          savePending={false}
          showPassword={showPassword}
          onToggleShowPassword={() => setShowPassword((current) => !current)}
          wifiInterfaces={wifiInterfaces}
          networkInterfaces={networkInterfaces}
          showActions={false}
        />
      </CardContent>
      <CardFooter className="justify-end gap-2 border-t pt-4">
        <Button variant="ghost" onClick={onSkip}>
          Pular por agora
        </Button>
        <Button onClick={handleSubmit(onNext)}>Continuar</Button>
      </CardFooter>
    </Card>
  );
}
