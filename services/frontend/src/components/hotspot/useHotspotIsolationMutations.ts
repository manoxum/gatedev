import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { HotspotCommRuleRequest } from "@/components/hotspot/hotspot-isolation-types";

function onIsolationError(error: unknown) {
  toast.error(error instanceof ApiError ? error.message : "Falha ao salvar isolamento");
}

export function useHotspotIsolationMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["hotspot", "isolation"] });
  };

  const setEnabled = useMutation({
    mutationFn: (enabled: boolean) => api.put("/hotspot/isolation", { enabled }),
    onSuccess: (_, enabled) => {
      toast.success(
        enabled
          ? "Isolamento ativado. Reinicie o hotspot para valer de fato."
          : "Isolamento desativado. Reinicie o hotspot para valer de fato.",
      );
      invalidate();
    },
    onError: onIsolationError,
  });

  const createRule = useMutation({
    mutationFn: (rule: HotspotCommRuleRequest) => api.post("/hotspot/isolation/rules", rule),
    onSuccess: () => {
      toast.success("Regra criada.");
      invalidate();
    },
    onError: onIsolationError,
  });

  const updateRule = useMutation({
    mutationFn: ({ id, rule }: { id: string; rule: HotspotCommRuleRequest }) =>
      api.patch(`/hotspot/isolation/rules/${encodeURIComponent(id)}`, rule),
    onSuccess: () => {
      toast.success("Regra salva.");
      invalidate();
    },
    onError: onIsolationError,
  });

  const removeRule = useMutation({
    mutationFn: (id: string) => api.del(`/hotspot/isolation/rules/${encodeURIComponent(id)}`),
    onSuccess: () => {
      toast.success("Regra removida.");
      invalidate();
    },
    onError: onIsolationError,
  });

  return { setEnabled, createRule, updateRule, removeRule };
}
