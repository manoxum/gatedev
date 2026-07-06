import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

export interface SetupServiceStatus {
  reachable: boolean;
  error?: string;
}

export interface SetupStatus {
  hotspotConfigured: boolean;
  services: Record<"postgres" | "mongo" | "minio", SetupServiceStatus>;
}

export function useSetupStatus(options: { enabled?: boolean } = {}) {
  return useQuery<SetupStatus>({
    queryKey: ["setup", "status"],
    queryFn: () => api.get<SetupStatus>("/setup/status"),
    retry: false,
    enabled: options.enabled ?? true,
  });
}
