import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { QuotaPeriod } from "@/components/hotspot/hotspot-limits-types";

export interface DeviceCreditConfig {
  enabled: boolean;
  rechargeAmountBytes: number | null;
  rechargePeriod: QuotaPeriod | null;
  plafondBytes: number | null;
}

function onCreditError(error: unknown) {
  toast.error(error instanceof ApiError ? error.message : "Falha ao salvar crédito");
}

export function useDeviceCreditMutations(mac: string) {
  const queryClient = useQueryClient();
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["hotspot", "devices", mac, "credit"] });

  const saveConfig = useMutation({
    mutationFn: (config: DeviceCreditConfig) => api.patch(`/hotspot/devices/${encodeURIComponent(mac)}/credit`, config),
    onSuccess: () => {
      toast.success("Configuração de crédito salva.");
      invalidate();
    },
    onError: onCreditError,
  });

  const recharge = useMutation({
    mutationFn: (amountBytes: number) =>
      api.post(`/hotspot/devices/${encodeURIComponent(mac)}/credit/recharge`, { amountBytes }),
    onSuccess: () => {
      toast.success("Crédito recarregado.");
      invalidate();
      queryClient.invalidateQueries({ queryKey: ["hotspot", "clients"] });
    },
    onError: onCreditError,
  });

  return { saveConfig, recharge };
}
