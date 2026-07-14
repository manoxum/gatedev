import type { FieldErrors, UseFormRegister } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import { RateUnitOptions } from "@/components/hotspot/RateUnitOptions";

// Fieldset de taxa (download/upload), extraido de HotspotRateQuotaFields
// para ser reusado tanto pelo limite global (via HotspotRateQuotaFields)
// quanto pelo limite de dispositivo/perfil (via HotspotLimitTypeFields) -
// taxa e sempre independente do tipo de limitacao. register/errors
// ficam frouxamente tipados de proposito, ja que este fieldset e
// reusado por schemas zod diferentes que so compartilham esses campos.
export function HotspotRateFields({
  register,
  errors,
}: {
  register: UseFormRegister<any>;
  errors: FieldErrors<any>;
}) {
  return (
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
          {errors.downloadRateValue && (
            <p className="text-sm text-destructive">{String(errors.downloadRateValue.message)}</p>
          )}
        </div>
        <div className="space-y-2">
          <Label htmlFor="uploadRateValue">Upload</Label>
          <div className="flex gap-2">
            <Input id="uploadRateValue" placeholder="sem limite" {...register("uploadRateValue")} />
            <SelectNative id="uploadRateUnit" className="w-24" {...register("uploadRateUnit")}>
              <RateUnitOptions />
            </SelectNative>
          </div>
          {errors.uploadRateValue && (
            <p className="text-sm text-destructive">{String(errors.uploadRateValue.message)}</p>
          )}
        </div>
      </div>
    </fieldset>
  );
}
