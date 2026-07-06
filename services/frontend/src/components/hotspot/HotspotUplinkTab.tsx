import type { FieldErrors, UseFormRegister } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TabsContent } from "@/components/ui/tabs";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";

interface HotspotUplinkTabProps {
  register: UseFormRegister<ConfigForm>;
  errors: FieldErrors<ConfigForm>;
}

export function HotspotUplinkTab({ register, errors }: HotspotUplinkTabProps) {
  return (
    <TabsContent value="uplink" className="mt-0">
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-muted-foreground">Uplink</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="BINDNET_UPLINK_INTERFACE">Interface virtual</Label>
            <Input id="BINDNET_UPLINK_INTERFACE" placeholder="bn-uplink" {...register("BINDNET_UPLINK_INTERFACE")} />
            {errors.BINDNET_UPLINK_INTERFACE && (
              <p className="text-sm text-destructive">{errors.BINDNET_UPLINK_INTERFACE.message}</p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="UPLINK_MONITOR_INTERVAL">Intervalo do monitor</Label>
            <Input id="UPLINK_MONITOR_INTERVAL" placeholder="10" {...register("UPLINK_MONITOR_INTERVAL")} />
            {errors.UPLINK_MONITOR_INTERVAL && (
              <p className="text-sm text-destructive">{errors.UPLINK_MONITOR_INTERVAL.message}</p>
            )}
          </div>
        </div>
      </fieldset>
    </TabsContent>
  );
}
