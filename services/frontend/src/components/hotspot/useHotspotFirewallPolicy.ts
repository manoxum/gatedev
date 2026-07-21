import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";

export interface FirewallPolicy {
  wanPolicy: "allow" | "deny";
  localPolicy: "allow" | "deny";
}

export function useHotspotFirewallPolicy() {
  return useQuery<FirewallPolicy>({
    queryKey: ["hotspot", "firewall", "policy"],
    queryFn: () => api.get<FirewallPolicy>("/hotspot/firewall/policy"),
  });
}

export function useHotspotFirewallPolicyMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (policy: FirewallPolicy) => api.put("/hotspot/firewall/policy", policy),
    onSuccess: () => {
      toast.success("Política padrão do firewall salva.");
      queryClient.invalidateQueries({ queryKey: ["hotspot", "firewall", "policy"] });
    },
    onError: (error: unknown) => toast.error(error instanceof ApiError ? error.message : "Falha ao salvar política"),
  });
}
