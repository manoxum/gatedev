import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { HotspotLimits } from "@/components/hotspot/hotspot-limits-types";

function onLimitsError(error: unknown) {
  toast.error(error instanceof ApiError ? error.message : "Falha ao salvar limite");
}

// Sem mutation de "remover limite" - o tipo de limitação (ilimitado/
// crédito/cota) é sempre salvo explicitamente via este PATCH, nunca
// removido para "voltar ao global" (o override nunca caía pro global
// mesmo antes, só pro perfil - ver RULE.md). Reset de cota por período
// é uma ação separada, ver useHotspotDeviceQuotaMutations.ts.
export function useDeviceLimitsMutation(mac: string) {
  const queryClient = useQueryClient();

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["hotspot", "devices", mac, "limits"] });

  const save = useMutation({
    mutationFn: (limits: HotspotLimits) => api.patch(`/hotspot/devices/${encodeURIComponent(mac)}/limits`, limits),
    onSuccess: () => {
      toast.success("Limite do dispositivo salvo.");
      invalidate();
    },
    onError: onLimitsError,
  });

  return { save };
}
