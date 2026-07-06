import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import { bytesToQuotaValue, RATE_UNIT_LABELS, type RateUnit } from "@/components/hotspot/hotspot-limits-types";
import { RateUnitOptions } from "@/components/hotspot/RateUnitOptions";
import { useDeviceCreditHistory } from "@/components/hotspot/useHotspotQueries";
import type { HotspotCreditEntryType } from "@/components/hotspot/hotspot-credit-types";

const ENTRY_LABELS: Record<HotspotCreditEntryType, string> = {
  manual_recharge: "Recarga manual",
  auto_recharge: "Recarga automática",
  debit: "Consumo",
};

// Extrato somente leitura da conta corrente de credito - cada recarga
// (manual/automatica) ou debito de trafego vira uma linha aqui, com o
// saldo resultante logo apos a movimentacao. O operador escolhe em qual
// unidade quer ver os valores (Kb/Mb/Gb bits, KB/MB/GB bytes) - a
// conversao e so de exibicao, o backend sempre guarda em bytes.
export function DeviceCreditHistoryCard({ mac }: { mac: string }) {
  const history = useDeviceCreditHistory(mac);
  const [unit, setUnit] = useState<RateUnit>("gbyte");
  const entries = history.data ?? [];
  const unitLabel = RATE_UNIT_LABELS[unit];

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <CardTitle>Extrato de crédito</CardTitle>
            <CardDescription>Últimas {entries.length} movimentações de saldo (recargas e consumo).</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Label htmlFor="creditHistoryUnit" className="text-xs text-muted-foreground">
              Unidade
            </Label>
            <SelectNative
              id="creditHistoryUnit"
              className="w-24"
              value={unit}
              onChange={(e) => setUnit(e.target.value as RateUnit)}
            >
              <RateUnitOptions />
            </SelectNative>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Data</TableHead>
              <TableHead>Tipo</TableHead>
              <TableHead className="text-right">Valor ({unitLabel})</TableHead>
              <TableHead className="text-right">Saldo após ({unitLabel})</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map((entry, index) => (
              <TableRow key={`${entry.createdAt}-${index}`}>
                <TableCell>{new Date(entry.createdAt).toLocaleString()}</TableCell>
                <TableCell>
                  <Badge variant={entry.entryType === "debit" ? "secondary" : "outline"}>
                    {ENTRY_LABELS[entry.entryType]}
                  </Badge>
                </TableCell>
                <TableCell className="text-right font-mono text-xs">
                  {entry.amountBytes >= 0 ? "+" : ""}
                  {bytesToQuotaValue(entry.amountBytes, unit).toFixed(2)}
                  {unitLabel}
                </TableCell>
                <TableCell className="text-right font-mono text-xs">
                  {bytesToQuotaValue(entry.balanceAfterBytes, unit).toFixed(2)}
                  {unitLabel}
                </TableCell>
              </TableRow>
            ))}
            {entries.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="text-center text-muted-foreground">
                  Nenhuma movimentação registrada ainda.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
