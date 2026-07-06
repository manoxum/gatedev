import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import {
  hotspotLimitsFormSchema,
  limitsToFormValues,
  formValuesToLimits,
  type HotspotLimitsFormValues,
} from "@/components/hotspot/hotspot-limits-schema";
import type { HotspotLimits } from "@/components/hotspot/hotspot-limits-types";
import { RateUnitOptions } from "@/components/hotspot/RateUnitOptions";

interface HotspotLimitsFormProps {
  value: HotspotLimits;
  onSubmit: (limits: HotspotLimits) => void;
  pending: boolean;
}

// Formulário genérico de limite (taxa valor+unidade + cota GB/período
// + taxa de throttle pós-cota), reusado tanto pelo limite global
// quanto pelo limite por dispositivo - só muda o que o container faz
// com o valor enviado em onSubmit.
export function HotspotLimitsForm({ value, onSubmit, pending }: HotspotLimitsFormProps) {
  const {
    register,
    handleSubmit,
    formState: { errors, isDirty },
  } = useForm<HotspotLimitsFormValues>({
    resolver: zodResolver(hotspotLimitsFormSchema),
    values: limitsToFormValues(value),
  });

  return (
    <form className="space-y-6" onSubmit={handleSubmit((values) => onSubmit(formValuesToLimits(values)))}>
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-muted-foreground">Taxa</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="downloadRateValue">Download</Label>
            <div className="flex gap-2">
              <Input id="downloadRateValue" placeholder="sem limite" {...register("downloadRateValue")} />
              <SelectNative id="downloadRateUnit" className="w-24" {...register("downloadRateUnit")}>
                <RateUnitOptions />
              </SelectNative>
            </div>
            {errors.downloadRateValue && <p className="text-sm text-destructive">{errors.downloadRateValue.message}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="uploadRateValue">Upload</Label>
            <div className="flex gap-2">
              <Input id="uploadRateValue" placeholder="sem limite" {...register("uploadRateValue")} />
              <SelectNative id="uploadRateUnit" className="w-24" {...register("uploadRateUnit")}>
                <RateUnitOptions />
              </SelectNative>
            </div>
            {errors.uploadRateValue && <p className="text-sm text-destructive">{errors.uploadRateValue.message}</p>}
          </div>
        </div>
      </fieldset>

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
            {errors.quotaValue && <p className="text-sm text-destructive">{errors.quotaValue.message}</p>}
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
        <legend className="text-sm font-medium text-muted-foreground">
          Taxa após estourar a cota (throttle)
        </legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="quotaThrottleDownloadValue">Download</Label>
            <div className="flex gap-2">
              <Input id="quotaThrottleDownloadValue" placeholder="sem throttle" {...register("quotaThrottleDownloadValue")} />
              <SelectNative id="quotaThrottleDownloadUnit" className="w-24" {...register("quotaThrottleDownloadUnit")}>
                <RateUnitOptions />
              </SelectNative>
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="quotaThrottleUploadValue">Upload</Label>
            <div className="flex gap-2">
              <Input id="quotaThrottleUploadValue" placeholder="sem throttle" {...register("quotaThrottleUploadValue")} />
              <SelectNative id="quotaThrottleUploadUnit" className="w-24" {...register("quotaThrottleUploadUnit")}>
                <RateUnitOptions />
              </SelectNative>
            </div>
          </div>
        </div>
      </fieldset>

      <Button type="submit" disabled={!isDirty || pending}>
        Salvar
      </Button>
    </form>
  );
}
