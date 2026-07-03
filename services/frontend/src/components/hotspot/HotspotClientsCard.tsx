import { useNavigate } from "react-router-dom";
import { Ban, RefreshCw, ScanSearch, Settings2, Undo2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

export interface HotspotClient {
  mac: string;
  ip: string;
  hostname: string;
  vendor?: string;
  deviceName?: string;
  osName?: string;
  confidence?: number;
  blocked?: boolean;
}

interface HotspotClientsCardProps {
  clients: HotspotClient[];
  running: boolean;
  identifyPendingMac?: string;
  blockPendingMac?: string;
  unblockPendingMac?: string;
  onIdentify: (mac: string) => void;
  onBlock: (mac: string) => void;
  onUnblock: (mac: string) => void;
}

export function HotspotClientsCard({
  clients,
  running,
  identifyPendingMac,
  blockPendingMac,
  unblockPendingMac,
  onIdentify,
  onBlock,
  onUnblock,
}: HotspotClientsCardProps) {
  const navigate = useNavigate();
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
              <TableHead>Perfil</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {clients.map((client) => (
              <TableRow key={client.mac}>
                <TableCell className="font-mono text-xs">{client.mac}</TableCell>
                <TableCell>
                  <div className="space-y-1">
                    <p className="font-medium">{client.ip}</p>
                    <p className="text-xs text-muted-foreground">{client.hostname || "sem hostname"}</p>
                  </div>
                </TableCell>
                <TableCell>
                  {client.deviceName || client.vendor || client.osName ? (
                    <div className="max-w-[320px] space-y-1">
                      <p className="truncate font-medium">{client.deviceName || client.vendor}</p>
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
                  <Badge variant={client.blocked ? "destructive" : "success"}>{client.blocked ? "bloqueado" : "ativo"}</Badge>
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
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={identifyPendingMac === client.mac}
                      onClick={() => onIdentify(client.mac)}
                    >
                      {identifyPendingMac === client.mac ? (
                        <RefreshCw className="h-4 w-4 animate-spin" />
                      ) : (
                        <ScanSearch className="h-4 w-4" />
                      )}
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
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={blockPendingMac === client.mac}
                        onClick={() => onBlock(client.mac)}
                      >
                        <Ban className="h-4 w-4" />
                        Bloquear
                      </Button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {running && clients.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-muted-foreground">
                  Nenhum cliente conectado.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
