import type { FieldErrors, UseFormRegister } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import { RateUnitOptions } from "@/components/hotspot/RateUnitOptions";
import { HotspotRateFields } from "@/components/hotspot/HotspotRateFields";

// Fieldsets de taxa/cota/throttle usados so pelo limite global
// (HotspotLimitsForm) - device/perfil usam o shape novo de tipo unico
// (ver HotspotLimitTypeFields.tsx). register/errors ficam frouxamente
// tipados (UseFormRegister<any>) de proposito, ja que este fieldset e
// reusado por schemas zod diferentes que so compartilham esse
// subconjunto de campos.
export function HotspotRateQuotaFields({
  register,
  errors,
}: {
  register: UseFormRegister<any>;
  errors: FieldErrors<any>;
}) {
  return (
    <>
      <HotspotRateFields register={register} errors={errors} />

      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-muted-foreground">Cota de dados</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="quotaValue">Cota</Label>
            <div className="flex gap-2">
              <Input id="quotaValue" placeholder="sem cota" {...register("quotaValue")} />
              <SelectNative id="quotaUnit" className="w-24" {...register("quotaUnit")}>
                <RateUnitOptions />
              </SelectNative>
            </div>
            {errors.quotaValue && <p className="text-sm text-destructive">{String(errors.quotaValue.message)}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="quotaPeriod">Período</Label>
            <SelectNative id="quotaPeriod" {...register("quotaPeriod")}>
              <option value="daily">Diário</option>
              <option value="weekly">Semanal</option>
              <option value="monthly">Mensal</option>
            </SelectNative>
          </div>
        </div>
      </fieldset>

      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-muted-foreground">Taxa após estourar a cota (throttle)</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="quotaThrottleDownloadValue">Download</Label>
            <div className="flex gap-2">
              <Input
                id="quotaThrottleDownloadValue"
                placeholder="sem throttle"
                {...register("quotaThrottleDownloadValue")}
              />
              <SelectNative id="quotaThrottleDownloadUnit" className="w-24" {...register("quotaThrottleDownloadUnit")}>
                <RateUnitOptions />
              </SelectNative>
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="quotaThrottleUploadValue">Upload</Label>
            <div className="flex gap-2">
              <Input
                id="quotaThrottleUploadValue"
                placeholder="sem throttle"
                {...register("quotaThrottleUploadValue")}
              />
              <SelectNative id="quotaThrottleUploadUnit" className="w-24" {...register("quotaThrottleUploadUnit")}>
                <RateUnitOptions />
              </SelectNative>
            </div>
          </div>
        </div>
      </fieldset>
    </>
  );
}
