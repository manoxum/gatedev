import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { HotspotKnownDevice } from "@/components/hotspot/useHotspotQueries";

interface HotspotKnownDevicesCardProps {
  devices: HotspotKnownDevice[];
}

function deviceLabel(device: HotspotKnownDevice) {
  return device.alias || device.deviceName || device.vendor || "dispositivo desconhecido";
}

// Somente leitura - todo dispositivo que ja apareceu na lista de
// clientes ao vivo alguma vez (ver recordDeviceSeen no backend),
// diferente das listas de "conectados agora" e "bloqueados".
export function HotspotKnownDevicesCard({ devices }: HotspotKnownDevicesCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Já conectados ({devices.length})</CardTitle>
        <CardDescription>Todo dispositivo que já se conectou ao hotspot alguma vez, mesmo desconectado agora.</CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Dispositivo</TableHead>
              <TableHead>MAC</TableHead>
              <TableHead>Primeira vez</TableHead>
              <TableHead>Última vez</TableHead>
              <TableHead className="text-right">Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {devices.map((device) => (
              <TableRow key={device.mac}>
                <TableCell>{deviceLabel(device)}</TableCell>
                <TableCell className="font-mono text-xs">{device.mac}</TableCell>
                <TableCell>{device.firstSeenAt ? new Date(device.firstSeenAt).toLocaleString() : "—"}</TableCell>
                <TableCell>{device.lastSeenAt ? new Date(device.lastSeenAt).toLocaleString() : "—"}</TableCell>
                <TableCell className="text-right">
                  <Badge variant={device.connected ? "success" : "outline"}>
                    {device.connected ? "conectado agora" : "desconectado"}
                  </Badge>
                </TableCell>
              </TableRow>
            ))}
            {devices.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-muted-foreground">
                  Nenhum dispositivo visto ainda.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
