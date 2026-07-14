import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Ban, ScanSearch, Settings2, Undo2, WifiOff } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatSpeedNow } from "@/components/hotspot/hotspot-limits-types";
import { useClientsStats } from "@/components/hotspot/useHotspotQueries";
import { DeviceIdentifyModal } from "@/components/hotspot/DeviceIdentifyModal";
import type { HotspotBlockMode } from "@/components/hotspot/useHotspotMutations";

// "manual" (bloqueio explicito do admin), "credit" (credito esgotado)
// ou "quota" (cota de dados esgotada) - ver deviceBlockReason em
// services/backend/hotspot_device_block_reason.go. "" ou ausente =
// nao bloqueado.
export type HotspotBlockReason = "manual" | "credit" | "quota" | "";

export interface HotspotClient {
  mac: string;
  ip: string;
  hostname: string;
  vendor?: string;
  deviceName?: string;
  osName?: string;
  confidence?: number;
  alias?: string;
  blocked?: boolean;
  blockReason?: HotspotBlockReason;
  profileId?: string;
  profileName?: string;
}

// blockStatusLabel traduz blocked/blockReason num rotulo+variante de
// Badge - reusado pela listagem (HotspotClientsCard) e pelo resumo do
// dispositivo (DeviceOverviewTab.tsx), pra nunca os dois textos
// divergirem.
export function blockStatusLabel(client: Pick<HotspotClient, "blocked" | "blockReason">): {
  label: string;
  variant: "success" | "destructive";
} {
  if (!client.blocked) return { label: "Ativo", variant: "success" };
  switch (client.blockReason) {
    case "credit":
      return { label: "Sem crédito", variant: "destructive" };
    case "quota":
      return { label: "Cota esgotada", variant: "destructive" };
    default:
      return { label: "Bloqueado", variant: "destructive" };
  }
}

interface HotspotClientsCardProps {
  clients: HotspotClient[];
  running: boolean;
  blockPendingMac?: string;
  unblockPendingMac?: string;
  onBlock: (mac: string, mode: HotspotBlockMode) => void;
  onUnblock: (mac: string) => void;
}

export function HotspotClientsCard({
  clients,
  running,
  blockPendingMac,
  unblockPendingMac,
  onBlock,
  onUnblock,
}: HotspotClientsCardProps) {
  const navigate = useNavigate();
  const stats = useClientsStats(running);
  const statsByMac = new Map(stats.data?.map((entry) => [entry.mac, entry]));
  const [identifyMac, setIdentifyMac] = useState<string | null>(null);
  const identifyClient = clients.find((client) => client.mac === identifyMac);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Clientes conectados ({clients.length})</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>MAC</TableHead>
              <TableHead>Endereço</TableHead>
              <TableHead>Identificação</TableHead>
              <TableHead>Perfil</TableHead>
              <TableHead>Velocidade</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {clients.map((client) => {
              const clientStats = statsByMac.get(client.mac);
              return (
              <TableRow key={client.mac}>
                <TableCell className="font-mono text-xs">{client.mac}</TableCell>
                <TableCell>
                  <div className="space-y-1">
                    <p className="font-medium">{client.ip}</p>
                    <p className="text-xs text-muted-foreground">{client.hostname || "sem hostname"}</p>
                  </div>
                </TableCell>
                <TableCell>
                  {client.alias || client.deviceName || client.vendor || client.osName ? (
                    <div className="max-w-[320px] space-y-1">
                      <p className="truncate font-medium">{client.alias || client.deviceName || client.vendor}</p>
                      <p className="truncate text-xs text-muted-foreground">
                        {[client.vendor, client.osName, client.confidence ? `${client.confidence}%` : ""]
                          .filter(Boolean)
                          .join(" · ")}
                      </p>
                    </div>
                  ) : (
                    <span className="text-sm text-muted-foreground">Sem identificação</span>
                  )}
                </TableCell>
                <TableCell>
                  <span className="text-sm">{client.profileName || "Padrão"}</span>
                </TableCell>
                <TableCell>
                  <div className="flex flex-col gap-0.5 text-xs text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <span className="h-1.5 w-1.5 rounded-full bg-primary" />
                      {formatSpeedNow(clientStats?.downloadBps ?? 0, "bit")}
                    </span>
                    <span className="flex items-center gap-1">
                      <span className="h-1.5 w-1.5 rounded-full bg-primary/40" />
                      {formatSpeedNow(clientStats?.uploadBps ?? 0, "bit")}
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  {(() => {
                    const status = blockStatusLabel(client);
                    return <Badge variant={status.variant}>{status.label}</Badge>;
                  })()}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => navigate(`/hotspot/devices/${encodeURIComponent(client.mac)}`)}
                    >
                      <Settings2 className="h-4 w-4" />
                      Ver detalhes
                    </Button>
                    <Button variant="outline" size="sm" onClick={() => setIdentifyMac(client.mac)}>
                      <ScanSearch className="h-4 w-4" />
                      Identificar
                    </Button>
                    {client.blocked ? (
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={unblockPendingMac === client.mac}
                        onClick={() => onUnblock(client.mac)}
                      >
                        <Undo2 className="h-4 w-4" />
                        Desbloquear
                      </Button>
                    ) : (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={blockPendingMac === client.mac}
                          onClick={() => onBlock(client.mac, "traffic")}
                        >
                          <WifiOff className="h-4 w-4" />
                          Cortar tráfego
                        </Button>
                        <Button
                          variant="destructive"
                          size="sm"
                          disabled={blockPendingMac === client.mac}
                          onClick={() => onBlock(client.mac, "deauth")}
                        >
                          <Ban className="h-4 w-4" />
                          Bloquear
                        </Button>
                      </>
                    )}
                  </div>
                </TableCell>
              </TableRow>
              );
            })}
            {running && clients.length === 0 && (
              <TableRow>
                <TableCell colSpan={7} className="text-center text-muted-foreground">
                  Nenhum cliente conectado.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
      <DeviceIdentifyModal
        client={identifyClient}
        open={identifyMac !== null}
        onOpenChange={(open) => !open && setIdentifyMac(null)}
      />
    </Card>
  );
}
