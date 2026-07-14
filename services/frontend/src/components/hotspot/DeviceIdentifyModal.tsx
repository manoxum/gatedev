import { useEffect, useState } from "react";
import { ScanSearch } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { useIdentifyDevice, useUpdateDeviceIdentity } from "@/components/hotspot/useHotspotMutations";

// Forma minima aceita pelo modal: tanto HotspotClient (lista de
// conectados) quanto HotspotKnownDevice (lista de todos os
// dispositivos) satisfazem essa interface.
export interface IdentifiableDevice {
  mac: string;
  alias?: string;
  vendor?: string;
  deviceName?: string;
  osName?: string;
}

interface DeviceIdentifyModalProps {
  client: IdentifiableDevice | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface FormState {
  alias: string;
  vendor: string;
  deviceName: string;
  osName: string;
}

function formStateFromClient(client: IdentifiableDevice | undefined): FormState {
  return {
    alias: client?.alias ?? "",
    vendor: client?.vendor ?? "",
    deviceName: client?.deviceName ?? "",
    osName: client?.osName ?? "",
  };
}

// Modal aberto pelo botão "Identificar" da lista de clientes
// conectados (HotspotClientsCard.tsx) ou da lista de todos os
// dispositivos (HotspotKnownDevicesCard.tsx): permite preencher
// alias/marca/modelo/SO à mão, ou pedir a identificação automática
// (fabricante via MAC, fingerprint DHCP, heurística de SO) e ajustar
// o resultado antes de salvar - ver
// PATCH /api/hotspot/devices/{mac}/identity.
export function DeviceIdentifyModal({ client, open, onOpenChange }: DeviceIdentifyModalProps) {
  const [form, setForm] = useState<FormState>(() => formStateFromClient(client));
  const identify = useIdentifyDevice();
  const updateIdentity = useUpdateDeviceIdentity();

  useEffect(() => {
    if (open) setForm(formStateFromClient(client));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, client?.mac]);

  if (!client) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Identificar dispositivo</DialogTitle>
          <DialogDescription>
            Preencha os campos à mão ou busque automaticamente pelo MAC {client.mac}.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <Button
            type="button"
            variant="outline"
            disabled={identify.isPending}
            onClick={() => identify.mutate(client.mac, { onSuccess: (info) => setForm(formStateFromClient(info)) })}
          >
            <ScanSearch className="h-4 w-4" />
            Buscar automaticamente
          </Button>

          <div className="space-y-2">
            <Label htmlFor="identify-alias">Alias</Label>
            <Input
              id="identify-alias"
              placeholder="apelido único"
              value={form.alias}
              onChange={(event) => setForm((current) => ({ ...current, alias: event.target.value }))}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="identify-vendor">Marca</Label>
              <Input
                id="identify-vendor"
                placeholder="sem marca"
                value={form.vendor}
                onChange={(event) => setForm((current) => ({ ...current, vendor: event.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="identify-deviceName">Modelo</Label>
              <Input
                id="identify-deviceName"
                placeholder="sem modelo"
                value={form.deviceName}
                onChange={(event) => setForm((current) => ({ ...current, deviceName: event.target.value }))}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="identify-osName">Sistema operacional</Label>
            <Input
              id="identify-osName"
              placeholder="sem SO"
              value={form.osName}
              onChange={(event) => setForm((current) => ({ ...current, osName: event.target.value }))}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            disabled={updateIdentity.isPending}
            onClick={() =>
              updateIdentity.mutate(
                { mac: client.mac, ...form },
                { onSuccess: () => onOpenChange(false) },
              )
            }
          >
            Salvar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
