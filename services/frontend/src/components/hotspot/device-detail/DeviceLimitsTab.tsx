import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { HotspotDeviceLimitsForm } from "@/components/hotspot/HotspotDeviceLimitsForm";
import { DeviceQuotaPeriodsCard } from "@/components/hotspot/device-detail/DeviceQuotaPeriodsCard";
import { LIMIT_TYPE_LABELS } from "@/components/hotspot/hotspot-limits-types";
import { useDeviceLimits } from "@/components/hotspot/useHotspotQueries";
import { useDeviceLimitsMutation } from "@/components/hotspot/useHotspotLimitMutations";

// O dispositivo so define a propria estrategia de limite quando o
// perfil vinculado e "customizado" (profileLimitType === "custom") -
// fora disso, o perfil decide o limite inteiro e o formulario vira um
// resumo so-leitura ("herdado do perfil X"), sem editar nada aqui. Ver
// effectiveDeviceLimits em services/backend/hotspot_profiles_apply.go.
export function DeviceLimitsTab({ mac }: { mac: string }) {
  const limits = useDeviceLimits(mac);
  const { save } = useDeviceLimitsMutation(mac);

  if (!limits.data) {
    return <div className="h-48 animate-pulse rounded-lg border bg-muted/30" />;
  }

  const customizable = limits.data.profileLimitType === "custom";

  return (
    <div className="space-y-6">
      {limits.data.limitType === "quota" && <DeviceQuotaPeriodsCard mac={mac} />}

      {customizable ? (
        <HotspotDeviceLimitsForm value={limits.data} onSubmit={(value) => save.mutate(value)} pending={save.isPending} />
      ) : (
        <Card>
          <CardContent className="space-y-3 pt-6">
            <p className="text-sm text-muted-foreground">
              Limite herdado do perfil <span className="font-medium text-foreground">{limits.data.profileName}</span>.
            </p>
            <Badge variant="secondary">{LIMIT_TYPE_LABELS[limits.data.limitType]}</Badge>
            <p className="text-xs text-muted-foreground">
              Para este dispositivo definir a própria estratégia de limite, vincule-o a um perfil do tipo
              "Customizado".
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
