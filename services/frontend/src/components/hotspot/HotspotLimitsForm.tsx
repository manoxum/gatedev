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

interface HotspotLimitsFormProps {
  value: HotspotLimits;
  onSubmit: (limits: HotspotLimits) => void;
  pending: boolean;
}

// Formulário genérico de limite (taxa Mbps + cota GB/período + taxa de
// throttle pós-cota), reusado tanto pelo limite global quanto pelo
// limite por dispositivo - só muda o que o container faz com o valor
// enviado em onSubmit.
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
        <legend className="text-sm font-medium text-muted-foreground">Taxa (Mbps)</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="downloadRateMbps">Download</Label>
            <Input id="downloadRateMbps" placeholder="sem limite" {...register("downloadRateMbps")} />
            {errors.downloadRateMbps && <p className="text-sm text-destructive">{errors.downloadRateMbps.message}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="uploadRateMbps">Upload</Label>
            <Input id="uploadRateMbps" placeholder="sem limite" {...register("uploadRateMbps")} />
            {errors.uploadRateMbps && <p className="text-sm text-destructive">{errors.uploadRateMbps.message}</p>}
          </div>
        </div>
      </fieldset>

      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-muted-foreground">Cota de dados</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="quotaGB">Cota (GB)</Label>
            <Input id="quotaGB" placeholder="sem cota" {...register("quotaGB")} />
            {errors.quotaGB && <p className="text-sm text-destructive">{errors.quotaGB.message}</p>}
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
            <Label htmlFor="quotaThrottleDownloadMbps">Download</Label>
            <Input id="quotaThrottleDownloadMbps" placeholder="sem throttle" {...register("quotaThrottleDownloadMbps")} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="quotaThrottleUploadMbps">Upload</Label>
            <Input id="quotaThrottleUploadMbps" placeholder="sem throttle" {...register("quotaThrottleUploadMbps")} />
          </div>
        </div>
      </fieldset>

      <Button type="submit" disabled={!isDirty || pending}>
        Salvar
      </Button>
    </form>
  );
}
