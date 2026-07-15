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
  wifiOpen: boolean;
  onWifiOpenChange: (open: boolean) => void;
}

export function HotspotWifiTab({
  register,
  errors,
  showPassword,
  onToggleShowPassword,
  wifiOpen,
  onWifiOpenChange,
}: HotspotWifiTabProps) {
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
              <Input
                id="WIFI_PASSWORD"
                type={showPassword ? "text" : "password"}
                disabled={wifiOpen}
                {...register("WIFI_PASSWORD")}
              />
              <button
                type="button"
                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground disabled:opacity-40"
                onClick={onToggleShowPassword}
                disabled={wifiOpen}
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {!wifiOpen && errors.WIFI_PASSWORD && (
              <p className="text-sm text-destructive">{errors.WIFI_PASSWORD.message}</p>
            )}
          </div>
        </div>
        <label className="flex items-start gap-2 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5 text-sm">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4 accent-primary"
            checked={wifiOpen}
            onChange={(event) => onWifiOpenChange(event.target.checked)}
          />
          <span>
            <span className="font-medium">Hotspot livre (sem senha)</span>
            <span className="block text-xs text-muted-foreground">
              Qualquer dispositivo próximo se conecta sem digitar senha. Enquanto marcada, a senha acima é ignorada.
            </span>
          </span>
        </label>
      </fieldset>
    </TabsContent>
  );
}
