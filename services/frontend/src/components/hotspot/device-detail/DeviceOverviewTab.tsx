import { Ban, Fingerprint, Laptop, Wifi } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { HotspotClient } from "@/components/hotspot/HotspotClientsCard";

function OverviewItem({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: string }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5">
      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="truncate text-sm font-semibold">{value}</p>
      </div>
    </div>
  );
}

export function DeviceOverviewTab({ client }: { client: HotspotClient }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <OverviewItem icon={Wifi} label="Endereço IP" value={client.ip || "desconhecido"} />
      <OverviewItem icon={Fingerprint} label="MAC" value={client.mac} />
      <OverviewItem
        icon={Laptop}
        label="Dispositivo"
        value={client.deviceName || client.vendor || "sem identificação"}
      />
      <OverviewItem icon={Ban} label="Status" value={client.blocked ? "Bloqueado" : "Ativo"} />
    </div>
  );
}
