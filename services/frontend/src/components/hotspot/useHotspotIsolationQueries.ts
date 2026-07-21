import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { HotspotCommRule, HotspotIsolationState } from "@/components/hotspot/hotspot-isolation-types";

export function useHotspotIsolationState() {
  return useQuery<HotspotIsolationState>({
    queryKey: ["hotspot", "isolation"],
    queryFn: () => api.get<HotspotIsolationState>("/hotspot/isolation"),
  });
}

export function useHotspotCommRules() {
  return useQuery<HotspotCommRule[]>({
    queryKey: ["hotspot", "isolation", "rules"],
    queryFn: () => api.get<HotspotCommRule[]>("/hotspot/isolation/rules"),
  });
}
