import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Ban, ScanSearch, Undo2, WifiOff } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { DeviceIdentifyModal } from "@/components/hotspot/DeviceIdentifyModal";
import type { HotspotKnownDevice } from "@/components/hotspot/useHotspotQueries";
import type { HotspotBlockMode } from "@/components/hotspot/useHotspotMutations";

interface HotspotKnownDevicesCardProps {
  devices: HotspotKnownDevice[];
  blockedMacs: Set<string>;
  blockPendingMac?: string;
  unblockPendingMac?: string;
  onBlock: (mac: string, mode: HotspotBlockMode) => void;
  onUnblock: (mac: string) => void;
}

function deviceLabel(device: HotspotKnownDevice) {
  return device.alias || device.deviceName || device.vendor || "dispositivo desconhecido";
}

// Mesmo padrão de listagem/ações da aba "Conectados"
// (HotspotClientsCard.tsx), mas cobrindo todo dispositivo que já
// apareceu na lista de clientes ao vivo alguma vez (ver
// recordDeviceSeen no backend), não só os conectados agora. Bloquear/
// cortar tráfego funcionam por MAC mesmo desconectado (bloqueio
// preventivo); "Identificar" exige o dispositivo conectado agora, já
// que depende de dados ao vivo (fingerprint DHCP) para um resultado
// útil.
export function HotspotKnownDevicesCard({
  devices,
  blockedMacs,
  blockPendingMac,
  unblockPendingMac,
  onBlock,
  onUnblock,
}: HotspotKnownDevicesCardProps) {
  const navigate = useNavigate();
  const [identifyMac, setIdentifyMac] = useState<string | null>(null);
  const identifyDevice = devices.find((device) => device.mac === identifyMac);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Todos os dispositivos ({devices.length})</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Dispositivo</TableHead>
              <TableHead className="hidden md:table-cell">MAC</TableHead>
              <TableHead className="hidden md:table-cell">Primeira vez</TableHead>
              <TableHead className="hidden sm:table-cell">Última vez</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {devices.map((device) => {
              const blocked = blockedMacs.has(device.mac);
              return (
                <TableRow
                  key={device.mac}
                  className="cursor-pointer"
                  onClick={() => navigate(`/hotspot/devices/${encodeURIComponent(device.mac)}`)}
                >
                  <TableCell>{deviceLabel(device)}</TableCell>
                  <TableCell className="hidden font-mono text-xs md:table-cell">{device.mac}</TableCell>
                  <TableCell className="hidden md:table-cell">
                    {device.firstSeenAt ? new Date(device.firstSeenAt).toLocaleString() : "—"}
                  </TableCell>
                  <TableCell className="hidden sm:table-cell">
                    {device.lastSeenAt ? new Date(device.lastSeenAt).toLocaleString() : "—"}
                  </TableCell>
                  <TableCell>
                    <Badge variant={blocked ? "destructive" : device.connected ? "success" : "outline"}>
                      {blocked ? "bloqueado" : device.connected ? "conectado agora" : "desconectado"}
                    </Badge>
                  </TableCell>
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    <div className="flex flex-wrap justify-end gap-2">
                      {device.connected && (
                        <Button variant="outline" size="sm" aria-label="Identificar" onClick={() => setIdentifyMac(device.mac)}>
                          <ScanSearch className="h-4 w-4" />
                          <span className="hidden sm:inline">Identificar</span>
                        </Button>
                      )}
                      {blocked ? (
                        <Button
                          variant="secondary"
                          size="sm"
                          aria-label="Desbloquear"
                          disabled={unblockPendingMac === device.mac}
                          onClick={() => onUnblock(device.mac)}
                        >
                          <Undo2 className="h-4 w-4" />
                          <span className="hidden sm:inline">Desbloquear</span>
                        </Button>
                      ) : (
                        <>
                          <Button
                            variant="outline"
                            size="sm"
                            aria-label="Cortar tráfego"
                            disabled={blockPendingMac === device.mac}
                            onClick={() => onBlock(device.mac, "traffic")}
                          >
                            <WifiOff className="h-4 w-4" />
                            <span className="hidden sm:inline">Cortar tráfego</span>
                          </Button>
                          <Button
                            variant="destructive"
                            size="sm"
                            aria-label="Bloquear"
                            disabled={blockPendingMac === device.mac}
                            onClick={() => onBlock(device.mac, "deauth")}
                          >
                            <Ban className="h-4 w-4" />
                            <span className="hidden sm:inline">Bloquear</span>
                          </Button>
                        </>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
            {devices.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-center text-muted-foreground">
                  Nenhum dispositivo visto ainda.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
      <DeviceIdentifyModal
        client={identifyDevice}
        open={identifyMac !== null}
        onOpenChange={(open) => !open && setIdentifyMac(null)}
      />
    </Card>
  );
}
