import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Plus } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { formatQuotaValue } from "@/components/hotspot/hotspot-limits-types";
import { HotspotVoucherIssueForm } from "@/components/hotspot/HotspotVoucherIssueForm";
import { useHotspotVoucherBatches } from "@/components/hotspot/useHotspotVoucherQueries";
import { useHotspotVoucherMutations } from "@/components/hotspot/useHotspotVoucherMutations";

// Gestao de vouchers (cartoes de recarga) - emitir um lote e listar os
// lotes ja emitidos. O detalhe de cada lote (codigos individuais,
// impressao em PDF) fica na pagina HotspotVoucherBatchDetail, aberta
// ao clicar numa linha. O resgate em si acontece so pelo proprio
// dispositivo, no portal de autoatendimento (ver src/pages/Portal.tsx).
export function HotspotVouchersCard() {
  const navigate = useNavigate();
  const batches = useHotspotVoucherBatches();
  const { issue } = useHotspotVoucherMutations();
  const [issuing, setIssuing] = useState(false);

  return (
    <Card>
      <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <CardTitle>Vouchers (cartões de recarga)</CardTitle>
        <Button size="sm" onClick={() => setIssuing(true)}>
          <Plus className="h-4 w-4" />
          Emitir lote de vouchers
        </Button>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Lote</TableHead>
              <TableHead className="hidden sm:table-cell">Valor por voucher</TableHead>
              <TableHead className="hidden sm:table-cell">Quantidade</TableHead>
              <TableHead>Situação</TableHead>
              <TableHead className="hidden md:table-cell">Nota</TableHead>
              <TableHead className="hidden md:table-cell">Emitido em</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(batches.data ?? []).map((batch) => (
              <TableRow
                key={batch.id}
                className="cursor-pointer"
                onClick={() => navigate(`/hotspot/vouchers/batches/${batch.id}`)}
              >
                <TableCell className="font-mono text-xs">{batch.id}</TableCell>
                <TableCell className="hidden text-sm sm:table-cell">
                  {formatQuotaValue(batch.amountBytes, batch.amountUnit)}
                </TableCell>
                <TableCell className="hidden text-sm sm:table-cell">{batch.quantity}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    <Badge variant="default">{batch.activeCount} ativo(s)</Badge>
                    {batch.redeemedCount > 0 && <Badge variant="secondary">{batch.redeemedCount} resgatado(s)</Badge>}
                    {batch.revokedCount > 0 && <Badge variant="outline">{batch.revokedCount} anulado(s)</Badge>}
                  </div>
                </TableCell>
                <TableCell className="hidden text-sm text-muted-foreground md:table-cell">{batch.note || "-"}</TableCell>
                <TableCell className="hidden text-sm text-muted-foreground md:table-cell">
                  {new Date(batch.createdAt).toLocaleString()}
                </TableCell>
              </TableRow>
            ))}
            {batches.data?.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-center text-muted-foreground">
                  Nenhum lote de vouchers emitido ainda.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>

      <Dialog open={issuing} onOpenChange={setIssuing}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Emitir lote de vouchers</DialogTitle>
            <DialogDescription>Os vouchers do lote ficam disponíveis na página de detalhe, com opção de impressão.</DialogDescription>
          </DialogHeader>
          <HotspotVoucherIssueForm
            pending={issue.isPending}
            onSubmit={(request) =>
              issue.mutate(request, {
                onSuccess: (response) => {
                  setIssuing(false);
                  navigate(`/hotspot/vouchers/batches/${response.batch.id}`);
                },
              })
            }
          />
        </DialogContent>
      </Dialog>
    </Card>
  );
}
