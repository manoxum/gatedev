import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Card, CardContent } from "@/components/ui/card";
import { SpeedGauge } from "@/components/ui/speed-gauge";
import { useDeviceLimits } from "@/components/hotspot/useHotspotQueries";

interface DeviceStats {
  downloadBps: number;
  uploadBps: number;
}

// O backend devolve downloadBps/uploadBps em BYTES por segundo (nome
// historico da API); os velocimetros trabalham em BITS por segundo
// (convenção de rede - Mbps/Gbps sempre são bits, nunca bytes).
export function toBitsPerSecond(bytesPerSecond: number) {
  return bytesPerSecond * 8;
}

function mbpsToBps(mbps: number | null | undefined) {
  return mbps ? mbps * 1_000_000 : null;
}

// Velocidade ao vivo do dispositivo - poll curto (2.5s) em
// /api/hotspot/devices/{mac}/stats, que ja devolve bytes/segundo
// prontos (o backend calcula o delta comparando duas leituras
// sucessivas dos contadores absolutos do worker). O teto do
// velocimetro usa o limite Mbps configurado do dispositivo, quando
// houver - senao autoescala.
export function DeviceSpeedCard({ mac }: { mac: string }) {
  const stats = useQuery<DeviceStats>({
    queryKey: ["hotspot", "devices", mac, "stats"],
    queryFn: () => api.get<DeviceStats>(`/hotspot/devices/${encodeURIComponent(mac)}/stats`),
    refetchInterval: 2500,
  });
  const limits = useDeviceLimits(mac);

  return (
    <Card>
      <CardContent className="flex flex-wrap justify-center gap-6 pt-6">
        <SpeedGauge
          valueBps={toBitsPerSecond(stats.data?.downloadBps ?? 0)}
          maxBps={mbpsToBps(limits.data?.downloadRateMbps)}
          label="Download"
          size="lg"
        />
        <SpeedGauge
          valueBps={toBitsPerSecond(stats.data?.uploadBps ?? 0)}
          maxBps={mbpsToBps(limits.data?.uploadRateMbps)}
          label="Upload"
          size="lg"
        />
      </CardContent>
    </Card>
  );
}
