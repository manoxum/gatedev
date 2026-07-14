import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import {
  hotspotLimitsFormSchema,
  limitsToFormValues,
  formValuesToLimits,
  type HotspotLimitsFormValues,
} from "@/components/hotspot/hotspot-device-limits-schema";
import type { HotspotLimits } from "@/components/hotspot/hotspot-limits-types";
import { HotspotLimitTypeFields } from "@/components/hotspot/HotspotLimitTypeFields";

interface HotspotDeviceLimitsFormProps {
  value: HotspotLimits;
  onSubmit: (limits: HotspotLimits) => void;
  pending: boolean;
}

// Formulário de limite de dispositivo (override) - taxa + tipo único
// (ilimitado/crédito/cota). A política de recarga de crédito não entra
// aqui (mora na aba "Crédito", DeviceCreditCard.tsx) - por isso
// showCreditRechargeFields=false. O formulário de perfil equivalente
// (HotspotProfileForm.tsx) reusa o mesmo HotspotLimitTypeFields com
// showCreditRechargeFields=true, já que perfil funde tudo num só
// formulário.
export function HotspotDeviceLimitsForm({ value, onSubmit, pending }: HotspotDeviceLimitsFormProps) {
  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors, isDirty },
  } = useForm<HotspotLimitsFormValues>({
    resolver: zodResolver(hotspotLimitsFormSchema),
    values: limitsToFormValues(value),
  });
  const limitType = watch("limitType");

  return (
    <form className="space-y-6" onSubmit={handleSubmit((values) => onSubmit(formValuesToLimits(values)))}>
      <HotspotLimitTypeFields
        register={register}
        errors={errors}
        limitType={limitType}
        onLimitTypeChange={(type) => setValue("limitType", type, { shouldDirty: true, shouldValidate: true })}
        showCreditRechargeFields={false}
      />

      <Button type="submit" disabled={!isDirty || pending}>
        Salvar
      </Button>
    </form>
  );
}
