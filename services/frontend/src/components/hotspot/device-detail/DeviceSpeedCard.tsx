import { useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp } from "lucide-react";
import { api } from "@/lib/api";
import { Card, CardContent } from "@/components/ui/card";

interface DeviceStats {
  downloadBps: number;
  uploadBps: number;
}

function formatMbps(bytesPerSecond: number) {
  const mbps = (bytesPerSecond * 8) / 1_000_000;
  return mbps.toFixed(mbps >= 10 ? 0 : 2);
}

// Velocidade ao vivo do dispositivo - poll curto (2.5s) em
// /api/hotspot/devices/{mac}/stats, que ja devolve bytes/segundo
// prontos (o backend calcula o delta comparando duas leituras
// sucessivas dos contadores absolutos do worker).
export function DeviceSpeedCard({ mac }: { mac: string }) {
  const stats = useQuery<DeviceStats>({
    queryKey: ["hotspot", "devices", mac, "stats"],
    queryFn: () => api.get<DeviceStats>(`/hotspot/devices/${encodeURIComponent(mac)}/stats`),
    refetchInterval: 2500,
  });

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Card>
        <CardContent className="flex items-center gap-3 pt-6">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
            <ArrowDown className="h-5 w-5" />
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Download</p>
            <p className="text-xl font-semibold leading-none">
              {formatMbps(stats.data?.downloadBps ?? 0)} <span className="text-sm font-normal">Mbps</span>
            </p>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardContent className="flex items-center gap-3 pt-6">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
            <ArrowUp className="h-5 w-5" />
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Upload</p>
            <p className="text-xl font-semibold leading-none">
              {formatMbps(stats.data?.uploadBps ?? 0)} <span className="text-sm font-normal">Mbps</span>
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
