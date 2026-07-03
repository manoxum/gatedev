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
import { bytesToGB, GIGABYTE } from "@/components/hotspot/hotspot-limits-types";
import { useDeviceCredit } from "@/components/hotspot/useHotspotQueries";
import { useDeviceCreditMutations, type DeviceCreditConfig } from "@/components/hotspot/useHotspotCreditMutations";

const optionalPositiveInt = z
  .string()
  .trim()
  .refine((value) => value === "" || (/^\d+$/.test(value) && Number(value) > 0), "Deve ser um número positivo");

const creditConfigSchema = z.object({
  enabled: z.boolean(),
  rechargeAmountGB: optionalPositiveInt,
  rechargePeriod: z.enum(["daily", "weekly", "monthly"]),
  plafondGB: optionalPositiveInt,
});
type CreditConfigForm = z.infer<typeof creditConfigSchema>;

export function DeviceCreditCard({ mac }: { mac: string }) {
  const [rechargeOpen, setRechargeOpen] = useState(false);
  const [rechargeGB, setRechargeGB] = useState("");
  const credit = useDeviceCredit(mac);
  const { saveConfig, recharge } = useDeviceCreditMutations(mac);

  const { register, handleSubmit, watch } = useForm<CreditConfigForm>({
    resolver: zodResolver(creditConfigSchema),
    values: credit.data
      ? {
          enabled: credit.data.enabled,
          rechargeAmountGB: credit.data.rechargeAmountBytes ? String(Math.round(bytesToGB(credit.data.rechargeAmountBytes))) : "",
          rechargePeriod: credit.data.rechargePeriod ?? "daily",
          plafondGB: credit.data.plafondBytes ? String(Math.round(bytesToGB(credit.data.plafondBytes))) : "",
        }
      : undefined,
  });
  const enabled = watch("enabled");

  if (!credit.data) {
    return <div className="h-48 animate-pulse rounded-lg border bg-muted/30" />;
  }

  function onSubmitConfig(values: CreditConfigForm) {
    const config: DeviceCreditConfig = {
      enabled: values.enabled,
      rechargeAmountBytes: values.rechargeAmountGB ? Number(values.rechargeAmountGB) * GIGABYTE : null,
      rechargePeriod: values.rechargeAmountGB ? values.rechargePeriod : null,
      plafondBytes: values.plafondGB ? Number(values.plafondGB) * GIGABYTE : null,
    };
    saveConfig.mutate(config);
  }

  function onSubmitRecharge() {
    const gb = Number(rechargeGB);
    if (!gb || gb <= 0) return;
    recharge.mutate(gb * GIGABYTE, { onSuccess: () => setRechargeOpen(false) });
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
        <div className="flex items-center gap-2">
          <input id="enabled" type="checkbox" className="h-4 w-4" {...register("enabled")} />
          <Label htmlFor="enabled">Exigir crédito para este dispositivo trafegar</Label>
        </div>

        {enabled && (
          <fieldset className="space-y-4">
            <legend className="text-sm font-medium text-muted-foreground">Recarga automática (opcional)</legend>
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="space-y-2">
                <Label htmlFor="rechargeAmountGB">Recarga por período (GB)</Label>
                <Input id="rechargeAmountGB" placeholder="só manual" {...register("rechargeAmountGB")} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="rechargePeriod">Período</Label>
                <SelectNative id="rechargePeriod" {...register("rechargePeriod")}>
                  <option value="daily">Diário</option>
                  <option value="weekly">Semanal</option>
                  <option value="monthly">Mensal</option>
                </SelectNative>
              </div>
              <div className="space-y-2">
                <Label htmlFor="plafondGB">Plafond - teto do saldo (GB)</Label>
                <Input id="plafondGB" placeholder="sem teto" {...register("plafondGB")} />
              </div>
            </div>
          </fieldset>
        )}

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
            <Label htmlFor="rechargeGB">Quantidade (GB)</Label>
            <Input id="rechargeGB" value={rechargeGB} onChange={(e) => setRechargeGB(e.target.value)} />
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
