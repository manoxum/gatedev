import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

export interface DnsRecord {
  hostname: string;
  address: string;
  loopbackOffset: number;
  createdAt: string;
}

export function useDnsQueries() {
  const config = useQuery<Record<string, string>>({
    queryKey: ["dns", "config"],
    queryFn: () => api.get<Record<string, string>>("/dns/config"),
  });

  const records = useQuery<DnsRecord[]>({
    queryKey: ["dns", "records"],
    queryFn: () => api.get<DnsRecord[]>("/dns/records"),
  });

  return { config, records };
}
