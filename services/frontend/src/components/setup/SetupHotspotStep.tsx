import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardFooter } from "@/components/ui/card";
import { HotspotConfigForm } from "@/components/hotspot/HotspotConfigForm";
import { configSchema, type ConfigForm } from "@/components/hotspot/hotspot-schema";
import { hotspotConfigDefaults } from "@/components/hotspot/hotspot-config-defaults";
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
    watch,
    setValue,
    formState: { errors, isDirty },
  } = useForm<ConfigForm>({ resolver: zodResolver(configSchema) });
  const wifiOpen = watch("WIFI_OPEN") === "true";

  const wifiInterfaces = interfaces.data?.filter((i) => i.type === "wifi") ?? [];
  const networkInterfaces = interfaces.data ?? [];

  useEffect(() => {
    if (initialData) {
      reset(initialData);
      return;
    }
    if (!config.data || !interfaces.data) return;
    reset(hotspotConfigDefaults(config.data, wifiInterfaces, "Bindnet"));
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
          wifiOpen={wifiOpen}
          onWifiOpenChange={(open) => setValue("WIFI_OPEN", open ? "true" : "false", { shouldDirty: true, shouldValidate: true })}
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
