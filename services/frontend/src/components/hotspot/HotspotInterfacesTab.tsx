import type { UseFormRegister } from "react-hook-form";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import { TabsContent } from "@/components/ui/tabs";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";

interface NetworkInterface {
  name: string;
  type: "wifi" | "other";
  state: string;
  speedMbps?: number;
}

interface HotspotInterfacesTabProps {
  register: UseFormRegister<ConfigForm>;
  wifiInterfaces: NetworkInterface[];
  networkInterfaces: NetworkInterface[];
}

export function interfaceLabel(i: NetworkInterface) {
  const speed = i.speedMbps ? `, ${i.speedMbps}Mbps` : "";
  return `${i.name} (${i.type}, ${i.state}${speed})`;
}

export function HotspotInterfacesTab({ register, wifiInterfaces, networkInterfaces }: HotspotInterfacesTabProps) {
  return (
    <TabsContent value="interfaces" className="mt-0">
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-muted-foreground">Interfaces</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="WIFI_INTERFACE">Interface Wi-Fi</Label>
            <SelectNative id="WIFI_INTERFACE" {...register("WIFI_INTERFACE")}>
              <option value="">Selecione...</option>
              {wifiInterfaces.map((i) => (
                <option key={i.name} value={i.name}>
                  {i.name} ({i.state})
                </option>
              ))}
            </SelectNative>
          </div>
          <div className="space-y-2">
            <Label htmlFor="INTERNET_INTERFACE">Interface de internet</Label>
            <SelectNative id="INTERNET_INTERFACE" {...register("INTERNET_INTERFACE")}>
              <option value="">Selecione...</option>
              <option value="auto">Automática (melhor disponível)</option>
              {networkInterfaces.map((i) => (
                <option key={i.name} value={i.name}>
                  {interfaceLabel(i)}
                </option>
              ))}
            </SelectNative>
          </div>
        </div>
      </fieldset>
    </TabsContent>
  );
}
