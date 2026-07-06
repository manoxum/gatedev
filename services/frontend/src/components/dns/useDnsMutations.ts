import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { DnsRecord } from "@/components/dns/useDnsQueries";

interface TestResponse {
  addresses?: string[];
  error?: string;
}

export function useDnsMutations() {
  const queryClient = useQueryClient();

  // Salva e já aplica (reinicia o DNS) numa única ação - separar em dois
  // passos só criava a falsa impressão de que "salvar" bastava, mas os
  // TLDs novos só valiam depois de "aplicar" mesmo assim.
  const saveTlds = useMutation({
    mutationFn: async (tlds: string[]) => {
      await api.patch("/dns/config", { DNS_LOCAL_TLDS: tlds.join(",") });
      await api.post("/dns/apply");
    },
    onSuccess: () => {
      toast.success("TLDs salvos e DNS reiniciado com os novos valores.");
      queryClient.invalidateQueries({ queryKey: ["dns", "config"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao salvar/aplicar"),
  });

  const test = useMutation({
    mutationFn: (hostname: string) => api.post<TestResponse>("/dns/test", { hostname }),
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao testar"),
  });

  const addRecord = useMutation({
    mutationFn: (hostname: string) => api.post<DnsRecord>("/dns/records", { hostname }),
    onSuccess: () => {
      toast.success("Registro adicionado.");
      queryClient.invalidateQueries({ queryKey: ["dns", "records"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao adicionar registro"),
  });

  const removeRecord = useMutation({
    mutationFn: (hostname: string) => api.del(`/dns/records/${encodeURIComponent(hostname)}`),
    onSuccess: () => {
      toast.success("Registro removido.");
      queryClient.invalidateQueries({ queryKey: ["dns", "records"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao remover registro"),
  });

  const clearRecords = useMutation({
    mutationFn: () => api.del("/dns/records"),
    onSuccess: () => {
      toast.success("Todos os registros foram removidos.");
      queryClient.invalidateQueries({ queryKey: ["dns", "records"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao limpar registros"),
  });

  return { saveTlds, test, addRecord, removeRecord, clearRecords };
}

export type { TestResponse };
