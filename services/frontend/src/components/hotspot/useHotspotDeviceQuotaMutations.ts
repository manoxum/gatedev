import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { QuotaPeriod } from "@/components/hotspot/hotspot-limits-types";

// Reset manual de UM período de cota (diário/semanal/mensal) - zera só
// aquele contador, sem afetar os outros períodos configurados nem o
// tipo de limitação do dispositivo. Ver POST .../quota/{period}/reset
// em services/backend/hotspot_device_quota.go.
export function useResetDeviceQuotaPeriod(mac: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (period: QuotaPeriod) => api.post(`/hotspot/devices/${encodeURIComponent(mac)}/quota/${period}/reset`),
    onSuccess: () => {
      toast.success("Cota resetada.");
      queryClient.invalidateQueries({ queryKey: ["hotspot", "devices", mac, "quota"] });
    },
    onError: (error: unknown) => {
      toast.error(error instanceof ApiError ? error.message : "Falha ao resetar cota");
    },
  });
}
