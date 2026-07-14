import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import {
  bytesToGB,
  GIGABYTE,
  quotaValueToBytes,
  type RateUnit,
} from "@/components/hotspot/hotspot-limits-types";
import { RateUnitOptions } from "@/components/hotspot/RateUnitOptions";
import { HotspotCreditConfigFields } from "@/components/hotspot/HotspotCreditConfigFields";
import { EmptyState } from "@/components/bindnets/EmptyState";
import { useDeviceCredit, useDeviceLimits } from "@/components/hotspot/useHotspotQueries";
import { useDeviceCreditMutations, type DeviceCreditConfig } from "@/components/hotspot/useHotspotCreditMutations";

const optionalPositiveInt = z
  .string()
  .trim()
  .refine((value) => value === "" || (/^\d+$/.test(value) && Number(value) > 0), "Deve ser um número positivo");

const creditConfigSchema = z.object({
  rechargeAmountGB: optionalPositiveInt,
  rechargePeriod: z.enum(["daily", "weekly", "monthly"]),
  plafondGB: optionalPositiveInt,
});
type CreditConfigForm = z.infer<typeof creditConfigSchema>;

// So mostra saldo/recarga/politica quando o tipo de limitação efetivo
// do dispositivo é "crédito" (ver hotspot-limits-types.ts) - fora
// disso, o saldo existe no banco (pode ter sobrado de uma configuração
// anterior) mas não é debitado nem confere no bloqueio, então exibi-lo
// aqui só confundiria o admin.
export function DeviceCreditCard({ mac }: { mac: string }) {
  const [rechargeOpen, setRechargeOpen] = useState(false);
  const [rechargeValue, setRechargeValue] = useState("");
  const [rechargeUnit, setRechargeUnit] = useState<RateUnit>("gbyte");
  const limits = useDeviceLimits(mac);
  const credit = useDeviceCredit(mac);
  const { saveConfig, recharge } = useDeviceCreditMutations(mac);

  const { register, handleSubmit } = useForm<CreditConfigForm>({
    resolver: zodResolver(creditConfigSchema),
    values: credit.data
      ? {
          rechargeAmountGB: credit.data.rechargeAmountBytes ? String(Math.round(bytesToGB(credit.data.rechargeAmountBytes))) : "",
          rechargePeriod: credit.data.rechargePeriod ?? "daily",
          plafondGB: credit.data.plafondBytes ? String(Math.round(bytesToGB(credit.data.plafondBytes))) : "",
        }
      : undefined,
  });

  if (!credit.data || !limits.data) {
    return <div className="h-48 animate-pulse rounded-lg border bg-muted/30" />;
  }

  if (limits.data.limitType !== "credit") {
    return (
      <EmptyState label={`Este dispositivo não usa crédito (tipo atual: ${limits.data.limitType === "quota" ? "Cota" : "Ilimitado"}). Mude para "Crédito" na aba Limites para usar saldo/recarga.`} />
    );
  }

  function onSubmitConfig(values: CreditConfigForm) {
    const config: DeviceCreditConfig = {
      rechargeAmountBytes: values.rechargeAmountGB ? Number(values.rechargeAmountGB) * GIGABYTE : null,
      rechargePeriod: values.rechargeAmountGB ? values.rechargePeriod : null,
      plafondBytes: values.plafondGB ? Number(values.plafondGB) * GIGABYTE : null,
    };
    saveConfig.mutate(config);
  }

  function onSubmitRecharge() {
    const value = Number(rechargeValue);
    if (!value || value <= 0) return;
    recharge.mutate(quotaValueToBytes(value, rechargeUnit), { onSuccess: () => setRechargeOpen(false) });
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between rounded-lg border bg-muted/30 px-4 py-3">
        <div>
          <p className="text-xs text-muted-foreground">Saldo atual</p>
          <p className="text-xl font-semibold">{bytesToGB(credit.data.balanceBytes).toFixed(2)}GB</p>
        </div>
        <div className="flex items-center gap-2">
          {credit.data.blockedByCredit && <Badge variant="destructive">bloqueado por falta de crédito</Badge>}
          <Button variant="outline" onClick={() => setRechargeOpen(true)}>
            Recarregar
          </Button>
        </div>
      </div>

      <form className="space-y-4" onSubmit={handleSubmit(onSubmitConfig)}>
        <HotspotCreditConfigFields register={register} />

        <Button type="submit" disabled={saveConfig.isPending}>
          Salvar
        </Button>
      </form>

      <Dialog open={rechargeOpen} onOpenChange={setRechargeOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Recarregar crédito</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="rechargeValue">Quantidade a incrementar no saldo</Label>
            <div className="flex gap-2">
              <Input id="rechargeValue" value={rechargeValue} onChange={(e) => setRechargeValue(e.target.value)} />
              <SelectNative
                id="rechargeUnit"
                className="w-24"
                value={rechargeUnit}
                onChange={(e) => setRechargeUnit(e.target.value as RateUnit)}
              >
                <RateUnitOptions />
              </SelectNative>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={onSubmitRecharge} disabled={recharge.isPending}>
              Confirmar recarga
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
