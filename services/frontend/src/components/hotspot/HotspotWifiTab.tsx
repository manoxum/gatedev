import type { FieldErrors, UseFormRegister } from "react-hook-form";
import { Eye, EyeOff } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TabsContent } from "@/components/ui/tabs";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";

interface HotspotWifiTabProps {
  register: UseFormRegister<ConfigForm>;
  errors: FieldErrors<ConfigForm>;
  showPassword: boolean;
  onToggleShowPassword: () => void;
}

export function HotspotWifiTab({ register, errors, showPassword, onToggleShowPassword }: HotspotWifiTabProps) {
  return (
    <TabsContent value="wifi" className="mt-0">
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-muted-foreground">Rede Wi-Fi</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="WIFI_SSID">SSID</Label>
            <Input id="WIFI_SSID" {...register("WIFI_SSID")} />
            {errors.WIFI_SSID && <p className="text-sm text-destructive">{errors.WIFI_SSID.message}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="WIFI_PASSWORD">Senha</Label>
            <div className="relative">
              <Input id="WIFI_PASSWORD" type={showPassword ? "text" : "password"} {...register("WIFI_PASSWORD")} />
              <button
                type="button"
                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground"
                onClick={onToggleShowPassword}
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {errors.WIFI_PASSWORD && <p className="text-sm text-destructive">{errors.WIFI_PASSWORD.message}</p>}
          </div>
        </div>
      </fieldset>
    </TabsContent>
  );
}
