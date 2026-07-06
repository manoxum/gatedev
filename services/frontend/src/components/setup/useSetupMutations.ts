import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";

interface FinishSetupInput {
  hotspot?: ConfigForm;
  dnsTlds?: string[];
}

// Salva e aplica de uma vez só tudo que foi coletado ao longo do
// assistente (hotspot e/ou DNS, o que não foi pulado) - passos
// individuais nunca chamam PATCH/apply sozinhos, só este hook, disparado
// pelo clique explícito do operador no último passo.
export function useFinishSetup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ hotspot, dnsTlds }: FinishSetupInput) => {
      if (hotspot) {
        await api.patch("/hotspot/config", hotspot);
        await api.post("/hotspot/apply");
      }
      if (dnsTlds) {
        await api.patch("/dns/config", { DNS_LOCAL_TLDS: dnsTlds.join(",") });
        await api.post("/dns/apply");
      }
    },
    // RequireAuth decide se redireciona pra /setup com base em
    // ["setup","status"] - o retorno aqui é essencial: sem esperar o
    // refetch terminar, o mutateAsync resolve com o cache ainda
    // desatualizado (hotspotConfigured antigo) e o RequireAuth que monta
    // na rota "/" manda de volta pro /setup antes do cache atualizar.
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["setup", "status"] }),
  });
}

export function setupErrorMessage(error: unknown, fallback: string) {
  return error instanceof ApiError && error.message ? error.message : fallback;
}
