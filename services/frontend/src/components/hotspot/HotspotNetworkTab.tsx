import type { FieldErrors, UseFormRegister } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TabsContent } from "@/components/ui/tabs";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";

interface HotspotNetworkTabProps {
  register: UseFormRegister<ConfigForm>;
  errors: FieldErrors<ConfigForm>;
}

export function HotspotNetworkTab({ register, errors }: HotspotNetworkTabProps) {
  return (
    <TabsContent value="network" className="mt-0">
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-muted-foreground">Rede do hotspot</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="HOTSPOT_GATEWAY">Gateway</Label>
            <Input id="HOTSPOT_GATEWAY" placeholder="192.168.12.1" {...register("HOTSPOT_GATEWAY")} />
            {errors.HOTSPOT_GATEWAY && <p className="text-sm text-destructive">{errors.HOTSPOT_GATEWAY.message}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="HOTSPOT_CIDR">Faixa CIDR</Label>
            <Input id="HOTSPOT_CIDR" placeholder="192.168.12.0/24" {...register("HOTSPOT_CIDR")} />
            {errors.HOTSPOT_CIDR && <p className="text-sm text-destructive">{errors.HOTSPOT_CIDR.message}</p>}
          </div>
          <div className="space-y-2 sm:col-span-2">
            <Label htmlFor="HOTSPOT_DNS_FALLBACKS">DNS públicos de fallback</Label>
            <Input id="HOTSPOT_DNS_FALLBACKS" placeholder="1.1.1.1,8.8.8.8" {...register("HOTSPOT_DNS_FALLBACKS")} />
          </div>
        </div>
      </fieldset>
    </TabsContent>
  );
}
