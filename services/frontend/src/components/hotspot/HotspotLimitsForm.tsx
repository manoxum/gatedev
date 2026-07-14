import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import {
  hotspotGlobalLimitsFormSchema,
  globalLimitsToFormValues,
  formValuesToGlobalLimits,
  type HotspotGlobalLimitsFormValues,
} from "@/components/hotspot/hotspot-limits-schema";
import type { HotspotGlobalLimits } from "@/components/hotspot/hotspot-limits-types";
import { HotspotRateQuotaFields } from "@/components/hotspot/HotspotRateQuotaFields";

interface HotspotLimitsFormProps {
  value: HotspotGlobalLimits;
  onSubmit: (limits: HotspotGlobalLimits) => void;
  pending: boolean;
}

// Formulário do limite global (taxa valor+unidade + cota GB/período +
// taxa de throttle pós-cota) - o limite de dispositivo/perfil usa o
// shape novo de tipo único (ver HotspotDeviceLimitsForm.tsx).
export function HotspotLimitsForm({ value, onSubmit, pending }: HotspotLimitsFormProps) {
  const {
    register,
    handleSubmit,
    formState: { errors, isDirty },
  } = useForm<HotspotGlobalLimitsFormValues>({
    resolver: zodResolver(hotspotGlobalLimitsFormSchema),
    values: globalLimitsToFormValues(value),
  });

  return (
    <form className="space-y-6" onSubmit={handleSubmit((values) => onSubmit(formValuesToGlobalLimits(values)))}>
      <HotspotRateQuotaFields register={register} errors={errors} />

      <Button type="submit" disabled={!isDirty || pending}>
        Salvar
      </Button>
    </form>
  );
}
