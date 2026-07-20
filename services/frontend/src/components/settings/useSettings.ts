import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { PanelSettings, PanelSettingsUpdate } from "@/components/settings/settings-types";

export function useSettings() {
  const queryClient = useQueryClient();

  const settings = useQuery<PanelSettings>({
    queryKey: ["settings"],
    queryFn: () => api.get<PanelSettings>("/settings"),
  });

  const save = useMutation({
    mutationFn: (update: PanelSettingsUpdate) => api.patch("/settings", update),
    onSuccess: () => {
      toast.success("Configurações salvas.");
      queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao salvar"),
  });

  return { settings, save };
}
