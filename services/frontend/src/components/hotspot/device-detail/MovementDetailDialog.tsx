import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { formatAutoScaleBytes, type ByteNature } from "@/components/hotspot/hotspot-limits-types";
import { useSessionConsumption } from "@/components/hotspot/useHotspotQueries";
import type { HotspotCreditEntryType, HotspotCreditHistoryEntry } from "@/components/hotspot/hotspot-credit-types";
import { SessionConsumptionChart } from "@/components/hotspot/device-detail/SessionConsumptionChart";

const ENTRY_LABELS: Record<HotspotCreditEntryType, string> = {
  manual_recharge: "Recarga manual",
  auto_recharge: "Recarga automática",
  voucher_redemption: "Voucher resgatado",
  session_active: "Sessão ativa",
  session_closed: "Sessão encerrada",
};

interface MovementDetailDialogProps {
  mac: string;
  entry: HotspotCreditHistoryEntry | null;
  nature: ByteNature;
  onOpenChange: (open: boolean) => void;
}

// Detalhe de uma linha da conta corrente (DeviceMovementsCard.tsx).
// Recarga/voucher mostra so os campos do proprio extrato; sessao
// (sessionId presente) tambem busca e renderiza o consumo bruto
// registrado no Mongo (grafico + tabela, ver SessionConsumptionDetail).
export function MovementDetailDialog({ mac, entry, nature, onOpenChange }: MovementDetailDialogProps) {
  return (
    <Dialog open={entry !== null} onOpenChange={onOpenChange}>
      {/* flex-col + header fora do scroll: so o corpo (abaixo) rola,
          nunca o titulo - antes o overflow-y-auto estava no
          DialogContent inteiro e arrastava o cabecalho junto. */}
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{entry ? ENTRY_LABELS[entry.entryType] : "Detalhe"}</DialogTitle>
        </DialogHeader>
        {entry && (
          <div className="flex-1 space-y-4 overflow-y-auto pr-1">
            <dl className="grid grid-cols-2 gap-3 text-sm">
              <div>
                <dt className="text-xs text-muted-foreground">Data</dt>
                <dd>{new Date(entry.createdAt).toLocaleString()}</dd>
              </div>
              <div>
                <dt className="text-xs text-muted-foreground">Valor</dt>
                <dd className="font-mono text-xs">
                  {entry.amountBytes >= 0 ? "+" : ""}
                  {formatAutoScaleBytes(entry.amountBytes, nature)}
                </dd>
              </div>
              {entry.balanceAfterBytes !== undefined && (
                <div>
                  <dt className="text-xs text-muted-foreground">Saldo após</dt>
                  <dd className="font-mono text-xs">{formatAutoScaleBytes(entry.balanceAfterBytes, nature)}</dd>
                </div>
              )}
              {entry.startedAt && (
                <div>
                  <dt className="text-xs text-muted-foreground">Início da sessão</dt>
                  <dd>{new Date(entry.startedAt).toLocaleString()}</dd>
                </div>
              )}
              <div>
                <dt className="text-xs text-muted-foreground">Fim da sessão</dt>
                <dd>{entry.endedAt ? new Date(entry.endedAt).toLocaleString() : "ainda conectado"}</dd>
              </div>
            </dl>
            {entry.sessionId !== undefined && <SessionConsumptionDetail mac={mac} sessionId={entry.sessionId} nature={nature} />}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function SessionConsumptionDetail({ mac, sessionId, nature }: { mac: string; sessionId: number; nature: ByteNature }) {
  const consumption = useSessionConsumption(mac, sessionId);
  const entries = consumption.data ?? [];

  return (
    <div className="space-y-4">
      <p className="text-xs font-medium text-muted-foreground">Consumo bruto registrado nesta sessão</p>
      {entries.length === 0 ? (
        <p className="text-center text-sm text-muted-foreground">Nenhum consumo registrado (ou trace já expirado).</p>
      ) : (
        <>
          <SessionConsumptionChart entries={entries} nature={nature} />
          {/* Tabela com altura maxima e scroll proprio, independente do
              corpo do dialogo - uma sessao longa nao deve empurrar o
              grafico/dl pra fora de vista. */}
          <div className="max-h-64 overflow-y-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Data</TableHead>
                  <TableHead className="text-right">Consumo</TableHead>
                  <TableHead className="text-right">Saldo após</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((entry, index) => (
                  <TableRow key={`${entry.createdAt}-${index}`}>
                    <TableCell>{new Date(entry.createdAt).toLocaleString()}</TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {formatAutoScaleBytes(entry.amountBytes, nature)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {formatAutoScaleBytes(entry.balanceAfterBytes, nature)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </>
      )}
    </div>
  );
}
