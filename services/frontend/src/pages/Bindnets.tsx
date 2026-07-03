import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ChevronRight, Fingerprint, Globe2, RadioTower, Server, ServerCog, Unplug } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import {
  formatSeen,
  nodePath,
  nodeTone,
  nodeLabel,
  remoteRoutes,
  serviceRowsForNode,
  unlinkPeerAddress,
  useMeshData,
  type BindnetNode,
} from "@/lib/mesh";
import { AddBindnetFab } from "@/components/bindnets/AddBindnetFab";
import { EmptyState } from "@/components/bindnets/EmptyState";
import { usePageHeader } from "@/hooks/usePageHeader";

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md border bg-card px-3 py-2">
      <div className="text-lg font-semibold leading-none">{value}</div>
      <div className="mt-1 text-xs text-muted-foreground">{label}</div>
    </div>
  );
}

export function BindnetsPage() {
  usePageHeader({ title: "Servidores Bindnet", description: "Nós, serviços e rotas descobertos pela malha." });

  const queryClient = useQueryClient();
  const [unlinkTarget, setUnlinkTarget] = useState<BindnetNode | null>(null);
  const mesh = useMeshData();
  const data = mesh.data;
  const nodes = data?.nodes ?? [];
  const visibleRemoteRoutes = data ? remoteRoutes(data) : [];
  const activeRoutes = visibleRemoteRoutes.filter((route) => route.state === "ok").length;
  const totalServices = data ? nodes.reduce((total, node) => total + serviceRowsForNode(node, data).length, 0) : 0;

  const unlinkPeer = useMutation({
    mutationFn: (node: BindnetNode) => unlinkPeerAddress(data?.config, node.address),
    onSuccess: () => {
      toast.success("Servidor desvinculado.");
      setUnlinkTarget(null);
      queryClient.invalidateQueries({ queryKey: ["dns", "config"] });
      queryClient.invalidateQueries({ queryKey: ["dns", "routes"] });
      queryClient.invalidateQueries({ queryKey: ["bindnets", "mesh"] });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Falha ao desvincular servidor"),
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div className="inline-flex items-center gap-2 rounded-md border bg-background px-3 py-1 text-xs font-medium text-muted-foreground">
          <RadioTower className="h-3.5 w-3.5" />
          Discover mesh
        </div>
        <div className="grid grid-cols-3 gap-2 sm:w-[420px]">
          <Metric label="Nós" value={nodes.length} />
          <Metric label="Serviços" value={totalServices} />
          <Metric label="Rotas ok" value={activeRoutes} />
        </div>
      </div>

      <div className="grid gap-3 lg:grid-cols-3">
        {mesh.isLoading &&
          Array.from({ length: 3 }).map((_, index) => (
            <div key={index} className="h-36 animate-pulse rounded-lg border bg-muted/30" />
          ))}
        {!mesh.isLoading && nodes.length === 0 && <EmptyState label="Nenhum servidor encontrado." />}
        {nodes.map((node) => {
          const services = data ? serviceRowsForNode(node, data).length : 0;
          const via = node.kind === "inferred" ? "aprendido por rota" : node.source;
          const domains = node.domains?.slice(0, 2).join(", ") || "sem domínio anunciado";
          const fingerprint = node.fingerprint ? node.fingerprint.slice(0, 12) : "sem fingerprint";
          return (
            <div
              key={node.id}
              className="group rounded-lg border bg-card p-4 text-card-foreground shadow-sm transition hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-md"
            >
              <Link to={nodePath(node.id)} className="block">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-center gap-3">
                    <div className={cn("flex h-10 w-10 items-center justify-center rounded-md border", nodeTone(node.kind))}>
                      {node.kind === "local" ? <ServerCog className="h-5 w-5" /> : <Server className="h-5 w-5" />}
                    </div>
                    <div className="min-w-0">
                      <p className="truncate font-semibold">{node.name}</p>
                      <p className="truncate text-xs text-muted-foreground">{node.address}</p>
                    </div>
                  </div>
                  <ChevronRight className="h-4 w-4 text-muted-foreground transition group-hover:translate-x-0.5" />
                </div>
              </Link>
              <div className="mt-5 flex items-center justify-between">
                <Badge variant="outline" className={cn("border", nodeTone(node.kind))}>
                  {nodeLabel(node.kind)}
                </Badge>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{services} serviços</span>
                  {node.kind === "direct" && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-muted-foreground hover:text-destructive"
                      onClick={() => setUnlinkTarget(node)}
                      aria-label={`Desvincular ${node.name}`}
                    >
                      <Unplug className="h-3.5 w-3.5" />
                    </Button>
                  )}
                </div>
              </div>
              <div className="mt-4 grid gap-2 text-xs text-muted-foreground">
                <div className="flex min-w-0 items-center gap-2">
                  <Globe2 className="h-3.5 w-3.5 shrink-0" />
                  <span className="truncate">{domains}</span>
                </div>
                <div className="flex min-w-0 items-center gap-2">
                  <Fingerprint className="h-3.5 w-3.5 shrink-0" />
                  <span className="truncate">{fingerprint}</span>
                </div>
              </div>
              <Separator className="my-4" />
              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <span>{via}</span>
                <span>{formatSeen(node.lastSeenAt)}</span>
              </div>
            </div>
          );
        })}
      </div>
      <AddBindnetFab config={data?.config} />
      <ConfirmDialog
        open={!!unlinkTarget}
        onOpenChange={(open) => !open && setUnlinkTarget(null)}
        title="Desvincular servidor"
        description={
          unlinkTarget
            ? `Remover ${unlinkTarget.address} dos peers diretos e aplicar a configuração de DNS agora.`
            : undefined
        }
        confirmLabel="Desvincular"
        variant="destructive"
        pending={unlinkPeer.isPending}
        onConfirm={() => unlinkTarget && unlinkPeer.mutate(unlinkTarget)}
      />
    </div>
  );
}
