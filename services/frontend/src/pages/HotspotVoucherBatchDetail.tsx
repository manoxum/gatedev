import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, Download, QrCode, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { EmptyState } from "@/components/bindnets/EmptyState";
import { formatQuotaValue } from "@/components/hotspot/hotspot-limits-types";
import { downloadVoucherBatchPdf } from "@/components/hotspot/hotspot-voucher-pdf";
import { HotspotVoucherQrDialog } from "@/components/hotspot/HotspotVoucherQrDialog";
import { useHotspotVoucherBatch, useHotspotVouchers } from "@/components/hotspot/useHotspotVoucherQueries";
import { useHotspotVoucherMutations } from "@/components/hotspot/useHotspotVoucherMutations";
import type { HotspotVoucher, HotspotVoucherStatus } from "@/components/hotspot/hotspot-voucher-types";
import { usePageHeader } from "@/hooks/usePageHeader";

const STATUS_LABELS: Record<HotspotVoucherStatus, string> = {
  active: "Ativo",
  redeemed: "Resgatado",
  revoked: "Anulado",
};

const STATUS_BADGE_VARIANT: Record<HotspotVoucherStatus, "default" | "secondary" | "outline"> = {
  active: "default",
  redeemed: "secondary",
  revoked: "outline",
};

// Detalhe de um lote de vouchers: lista os codigos individuais e
// permite baixar o lote inteiro em PDF ou anular um voucher ainda nao
// resgatado (ver services/backend/hotspot_voucher_batches.go).
export function HotspotVoucherBatchDetailPage() {
  const { id: batchId } = useParams();
  const navigate = useNavigate();
  const batch = useHotspotVoucherBatch(batchId ?? "");
  const vouchers = useHotspotVouchers(undefined, batchId);
  const { revoke } = useHotspotVoucherMutations();
  const [revoking, setRevoking] = useState<HotspotVoucher | null>(null);
  const [qrCode, setQrCode] = useState<string | null>(null);

  usePageHeader({ title: batchId ? `Lote ${batchId}` : "Lote de vouchers" });

  if (batch.isError) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" className="px-0" onClick={() => navigate("/hotspot")}>
          <ArrowLeft className="h-4 w-4" />
          Hotspot
        </Button>
        <EmptyState label="Lote não encontrado." />
      </div>
    );
  }

  if (!batch.data) {
    return <div className="h-80 animate-pulse rounded-lg border bg-muted/30" />;
  }

  const amountLabel = formatQuotaValue(batch.data.amountBytes, batch.data.amountUnit);

  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <Button variant="ghost" className="px-0" onClick={() => navigate("/hotspot")}>
          <ArrowLeft className="h-4 w-4" />
          Hotspot
        </Button>
        <div>
          <h1 className="text-2xl font-semibold">Lote de vouchers</h1>
          <p className="font-mono text-sm text-muted-foreground">{batch.data.id}</p>
        </div>
      </div>

      <Card>
        <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <CardTitle>
            {batch.data.quantity} voucher(s) de {amountLabel}
            {batch.data.note ? ` · ${batch.data.note}` : ""}
          </CardTitle>
          <Button
            size="sm"
            variant="outline"
            disabled={!vouchers.data?.length}
            onClick={() => vouchers.data && downloadVoucherBatchPdf(batch.data, vouchers.data)}
          >
            <Download className="h-4 w-4" />
            Baixar PDF
          </Button>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Código</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="hidden sm:table-cell">Resgatado por</TableHead>
                <TableHead className="text-right">Ações</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(vouchers.data ?? []).map((voucher) => (
                <TableRow key={voucher.code}>
                  <TableCell className="font-mono text-xs">{voucher.code}</TableCell>
                  <TableCell>
                    <Badge variant={STATUS_BADGE_VARIANT[voucher.status]}>{STATUS_LABELS[voucher.status]}</Badge>
                  </TableCell>
                  <TableCell className="hidden font-mono text-xs sm:table-cell">
                    {voucher.redeemedByMac ? `${voucher.redeemedByMac} · ${new Date(voucher.redeemedAt!).toLocaleString()}` : "-"}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-2">
                      <Button variant="outline" size="sm" onClick={() => setQrCode(voucher.code)}>
                        <QrCode className="h-4 w-4" />
                      </Button>
                      {voucher.status === "active" && (
                        <Button variant="outline" size="sm" onClick={() => setRevoking(voucher)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {vouchers.data?.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-muted-foreground">
                    Nenhum voucher neste lote.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => !open && setRevoking(null)}
        title="Anular voucher"
        description={`O voucher "${revoking?.code}" não poderá mais ser resgatado. Continuar?`}
        confirmLabel="Anular"
        variant="destructive"
        pending={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking.code, { onSuccess: () => setRevoking(null) })}
      />

      <HotspotVoucherQrDialog code={qrCode} onOpenChange={(open) => !open && setQrCode(null)} />
    </div>
  );
}
