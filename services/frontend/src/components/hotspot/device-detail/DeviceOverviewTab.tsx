import { useState } from "react";
import { Ban, Check, Fingerprint, Laptop, Tag, Wifi } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useUpdateDeviceIdentity } from "@/components/hotspot/useHotspotMutations";
import { DeviceProfileSelect } from "@/components/hotspot/device-detail/DeviceProfileSelect";
import { blockStatusLabel, type HotspotClient } from "@/components/hotspot/HotspotClientsCard";
import { WifiSignalIndicator } from "@/components/hotspot/WifiSignalIndicator";

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

// Alias e um apelido único definido pelo admin para o dispositivo
// (distinto do deviceName inferido automaticamente por heurística em
// "Identificar") - PATCH /api/hotspot/devices/{mac}/alias, ver
// useSetDeviceAlias em useHotspotMutations.ts.
function AliasItem({ mac, alias }: { mac: string; alias?: string }) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(alias ?? "");
  const updateIdentity = useUpdateDeviceIdentity();

  if (!editing) {
    return (
      <div className="flex items-center gap-3 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <Tag className="h-4 w-4" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-xs text-muted-foreground">Alias</p>
          <p className="truncate text-sm font-semibold">{alias || "sem alias"}</p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            setValue(alias ?? "");
            setEditing(true);
          }}
        >
          Editar
        </Button>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5">
      <Input
        autoFocus
        value={value}
        placeholder="sem alias"
        onChange={(event) => setValue(event.target.value)}
        onKeyDown={(event) => event.key === "Escape" && setEditing(false)}
      />
      <Button
        size="sm"
        disabled={updateIdentity.isPending}
        onClick={() =>
          updateIdentity.mutate(
            { mac, alias: value.trim() },
            { onSuccess: () => setEditing(false) },
          )
        }
      >
        <Check className="h-4 w-4" />
      </Button>
    </div>
  );
}

export function DeviceOverviewTab({ client, online }: { client: HotspotClient; online: boolean }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <OverviewItem icon={Wifi} label="Endereço IP" value={client.ip || "desconhecido"} />
      <OverviewItem icon={Fingerprint} label="MAC" value={client.mac} />
      <OverviewItem
        icon={Laptop}
        label="Dispositivo"
        value={client.deviceName || client.vendor || "sem identificação"}
      />
      <OverviewItem icon={Ban} label="Status" value={blockStatusLabel(client, online).label} />
      <WifiSignalIndicator dbm={client.signalDbm} size="lg" />
      <AliasItem mac={client.mac} alias={client.alias} />
      <DeviceProfileSelect mac={client.mac} profileId={client.profileId} />
    </div>
  );
}
