import { useState } from "react";
import { QrCode } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { HotspotQuotaPeriodBars } from "@/components/hotspot/HotspotQuotaPeriodBars";
import { bytesToGB } from "@/components/hotspot/hotspot-limits-types";
import { usePortalMe } from "@/components/portal/usePortalQueries";
import { useRedeemVoucher } from "@/components/portal/usePortalMutations";
import { PortalVoucherQrScanner } from "@/components/portal/PortalVoucherQrScanner";
import { ApiError } from "@/lib/api";

// Pagina de autoatendimento - unica rota publica alem de /login (ver
// src/App.tsx), sem AppLayout/usePageHeader (mesmo motivo de Login.tsx:
// nao ha contexto do outlet fora de RequireAuth). O dispositivo e
// identificado pelo IP de origem no backend (services/backend/
// hotspot_portal.go) - nunca por login nem por um MAC digitado aqui.
export function PortalPage() {
  const me = usePortalMe();
  const redeem = useRedeemVoucher();
  const [code, setCode] = useState("");
  const [scannerOpen, setScannerOpen] = useState(false);

  function onRedeem(value = code) {
    if (!value.trim()) return;
    redeem.mutate(value.trim().toUpperCase(), { onSuccess: () => setCode("") });
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Minha conexão</CardTitle>
          <CardDescription>Veja seu saldo e resgate um cartão de recarga.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {me.isLoading && <div className="h-40 animate-pulse rounded-lg border bg-muted/30" />}

          {me.isError && (
            <div className="space-y-3 text-center">
              <p className="text-sm text-destructive">
                {me.error instanceof ApiError && me.error.status === 409
                  ? "Não foi possível identificar seu dispositivo - reconecte-se ao Wi-Fi e tente novamente."
                  : "Não foi possível carregar sua conexão. Tente novamente em instantes."}
              </p>
              <Button variant="outline" onClick={() => me.refetch()}>
                Tentar novamente
              </Button>
            </div>
          )}

          {me.data && (
            <>
              <div className="flex items-center justify-between rounded-lg border bg-muted/30 px-4 py-3">
                <div>
                  <p className="text-xs text-muted-foreground">{me.data.alias || me.data.mac}</p>
                  <p className="text-xl font-semibold">
                    {me.data.limitType === "credit"
                      ? `${bytesToGB(me.data.balanceBytes).toFixed(2)}GB de saldo`
                      : "Sem cobrança de crédito"}
                  </p>
                </div>
                {me.data.blockedByCredit && <Badge variant="destructive">sem saldo</Badge>}
              </div>

              {me.data.limitType === "quota" && <HotspotQuotaPeriodBars periods={me.data.quotaPeriods ?? []} />}

              <div className="space-y-2">
                <Label htmlFor="voucherCode">Código do cartão de recarga</Label>
                <div className="flex gap-2">
                  <Input
                    id="voucherCode"
                    placeholder="XXXX-XXXX-XXXX"
                    value={code}
                    onChange={(event) => setCode(event.target.value)}
                  />
                  <Button variant="outline" size="icon" onClick={() => setScannerOpen(true)} aria-label="Ler QR code">
                    <QrCode className="h-4 w-4" />
                  </Button>
                  <Button onClick={() => onRedeem()} disabled={redeem.isPending || !code.trim()}>
                    Resgatar
                  </Button>
                </div>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <PortalVoucherQrScanner
        open={scannerOpen}
        onOpenChange={setScannerOpen}
        onScan={(scanned) => {
          setCode(scanned);
          onRedeem(scanned);
        }}
      />
    </div>
  );
}
