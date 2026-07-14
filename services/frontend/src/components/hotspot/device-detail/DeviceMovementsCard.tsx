import { useMemo, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import { formatAutoScaleBytes, type ByteNature } from "@/components/hotspot/hotspot-limits-types";
import { useDeviceCreditHistory } from "@/components/hotspot/useHotspotQueries";
import { creditEntryKind, type HotspotCreditEntryType, type HotspotCreditHistoryEntry } from "@/components/hotspot/hotspot-credit-types";
import { MovementDetailDialog } from "@/components/hotspot/device-detail/MovementDetailDialog";

const ENTRY_LABELS: Record<HotspotCreditEntryType, string> = {
  manual_recharge: "Recarga manual",
  auto_recharge: "Recarga automática",
  voucher_redemption: "Voucher resgatado",
  session_active: "Sessão ativa",
  session_closed: "Sessão encerrada",
};

type MovementFilter = "all" | "credit" | "debit" | HotspotCreditEntryType;

const FILTER_OPTIONS: { value: MovementFilter; label: string }[] = [
  { value: "all", label: "Todas as movimentações" },
  { value: "credit", label: "Só créditos" },
  { value: "debit", label: "Só débitos" },
  { value: "manual_recharge", label: ENTRY_LABELS.manual_recharge },
  { value: "auto_recharge", label: ENTRY_LABELS.auto_recharge },
  { value: "voucher_redemption", label: ENTRY_LABELS.voucher_redemption },
  { value: "session_active", label: ENTRY_LABELS.session_active },
  { value: "session_closed", label: ENTRY_LABELS.session_closed },
];

function matchesFilter(entry: HotspotCreditHistoryEntry, filter: MovementFilter) {
  if (filter === "all") return true;
  if (filter === "credit" || filter === "debit") return creditEntryKind(entry.entryType) === filter;
  return entry.entryType === filter;
}

// Conta corrente de credito - mescla recarga manual/automatica/voucher
// (credito) com toda sessao de conexao, ativa ou encerrada (debito, ver
// hotspot_sessions.go) numa unica lista ordenada por data, filtravel
// por tipo. Uma linha de sessao e clicavel: abre o detalhe de consumo
// daquela sessao (busca no Mongo, pode estar vazio se o trace ja
// expirou pela retencao configurada). Cada valor escolhe sozinho a
// grandeza (B/KB/MB/GB/TB) pelo proprio tamanho - ver autoScaleBytes -
// em vez de uma unidade fixa pra tabela inteira, que faria um consumo
// pequeno aparecer como "0.00GB" do lado de uma recarga grande. O
// operador so escolhe a natureza (bit ou byte), nunca a escala.
export function DeviceMovementsCard({ mac }: { mac: string }) {
  const history = useDeviceCreditHistory(mac);
  const [nature, setNature] = useState<ByteNature>("byte");
  const [filter, setFilter] = useState<MovementFilter>("all");
  const [selectedEntry, setSelectedEntry] = useState<HotspotCreditHistoryEntry | null>(null);
  const allEntries = history.data ?? [];
  const entries = useMemo(() => allEntries.filter((entry) => matchesFilter(entry, filter)), [allEntries, filter]);

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <CardTitle>Movimentações</CardTitle>
            <CardDescription>Créditos (carregamento, recarga, voucher) e débitos (sessões) da conta.</CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-4">
            <div className="flex items-center gap-2">
              <Label htmlFor="movementsFilter" className="text-xs text-muted-foreground">
                Filtrar
              </Label>
              <SelectNative
                id="movementsFilter"
                className="w-48"
                value={filter}
                onChange={(e) => setFilter(e.target.value as MovementFilter)}
              >
                {FILTER_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </SelectNative>
            </div>
            <div className="flex items-center gap-2">
              <Label htmlFor="movementsNature" className="text-xs text-muted-foreground">
                Grandeza
              </Label>
              <SelectNative
                id="movementsNature"
                className="w-24"
                value={nature}
                onChange={(e) => setNature(e.target.value as ByteNature)}
              >
                <option value="byte">Bytes</option>
                <option value="bit">Bits</option>
              </SelectNative>
            </div>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Data</TableHead>
              <TableHead>Tipo</TableHead>
              <TableHead className="text-right">Valor</TableHead>
              <TableHead className="text-right">Saldo após</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map((entry, index) => (
              <TableRow key={`${entry.createdAt}-${index}`} className="cursor-pointer" onClick={() => setSelectedEntry(entry)}>
                <TableCell>{new Date(entry.createdAt).toLocaleString()}</TableCell>
                <TableCell>
                  <Badge variant={creditEntryKind(entry.entryType) === "debit" ? "secondary" : "outline"}>
                    {ENTRY_LABELS[entry.entryType]}
                  </Badge>
                </TableCell>
                <TableCell className="text-right font-mono text-xs">
                  {entry.amountBytes >= 0 ? "+" : ""}
                  {formatAutoScaleBytes(entry.amountBytes, nature)}
                </TableCell>
                <TableCell className="text-right font-mono text-xs">
                  {entry.balanceAfterBytes !== undefined ? formatAutoScaleBytes(entry.balanceAfterBytes, nature) : "—"}
                </TableCell>
              </TableRow>
            ))}
            {entries.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="text-center text-muted-foreground">
                  Nenhuma movimentação encontrada.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
      <MovementDetailDialog
        mac={mac}
        entry={selectedEntry}
        nature={nature}
        onOpenChange={(open) => !open && setSelectedEntry(null)}
      />
    </Card>
  );
}
