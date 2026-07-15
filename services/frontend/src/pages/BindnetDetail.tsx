import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft, Cable, Globe2, LayoutGrid, Route, Server, ServerCog, ShieldCheck, Unplug } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api, ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  metricCards,
  neighborRows,
  nodeTone,
  remoteRoutes,
  routesViaNode,
  serviceRowsForNode,
  unlinkPeerAddress,
  useMeshData,
} from "@/lib/mesh";
import { EmptyState } from "@/components/bindnets/EmptyState";
import { BindnetCaSection } from "@/components/bindnets/BindnetCaSection";
import { BindnetOverviewTab } from "@/components/bindnets/BindnetOverviewTab";
import { BindnetServicesTab } from "@/components/bindnets/BindnetServicesTab";
import { BindnetNeighborsTab } from "@/components/bindnets/BindnetNeighborsTab";
import { BindnetRoutesTab } from "@/components/bindnets/BindnetRoutesTab";
import { usePageHeader } from "@/hooks/usePageHeader";
import { useUrlTab } from "@/hooks/useUrlTab";
import { useState } from "react";

export function BindnetDetailPage() {
  const { nodeId } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [unlinkOpen, setUnlinkOpen] = useState(false);
  const [tab, setTab] = useUrlTab("overview");
  const mesh = useMeshData();
  const id = nodeId ? decodeURIComponent(nodeId) : "";
  const data = mesh.data;
  const node = data?.nodes.find((candidate) => candidate.id === id);

  usePageHeader({ title: node?.name ?? "Servidores Bindnet", description: node?.address });

  const forgetRoute = useMutation({
    mutationFn: (domain: string) => api.del(`/dns/routes/${domain}`),
    onSuccess: () => {
      toast.success("Rota removida.");
      queryClient.invalidateQueries({ queryKey: ["bindnets", "mesh"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao remover rota"),
  });

  const unlinkPeer = useMutation({
    mutationFn: () => {
      if (!node) throw new Error("Servidor Bindnet não encontrado.");
      return unlinkPeerAddress(data?.config, node.address);
    },
    onSuccess: () => {
      toast.success("Servidor desvinculado.");
      setUnlinkOpen(false);
      queryClient.invalidateQueries({ queryKey: ["dns", "config"] });
      queryClient.invalidateQueries({ queryKey: ["dns", "routes"] });
      queryClient.invalidateQueries({ queryKey: ["bindnets", "mesh"] });
      navigate("/bindnets");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Falha ao desvincular servidor"),
  });

  if (!mesh.isLoading && !node) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" onClick={() => navigate("/bindnets")}>
          <ArrowLeft className="h-4 w-4" />
          Voltar
        </Button>
        <EmptyState label="Servidor Bindnet não encontrado." />
      </div>
    );
  }

  if (!data || !node) {
    return <div className="h-80 animate-pulse rounded-lg border bg-muted/30" />;
  }

  const services = serviceRowsForNode(node, data);
  const neighbors = neighborRows(node, data);
  const routed = routesViaNode(node, data.routes);
  const visibleRoutes = node.kind === "direct" ? routed : remoteRoutes(data);
  const metrics = metricCards(node, data);

  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <Button variant="ghost" className="px-0" onClick={() => navigate("/bindnets")}>
          <ArrowLeft className="h-4 w-4" />
          Bindnets
        </Button>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <div className={cn("flex h-14 w-14 shrink-0 items-center justify-center rounded-lg border", nodeTone(node.kind))}>
              {node.kind === "local" ? <ServerCog className="h-7 w-7" /> : <Server className="h-7 w-7" />}
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-2xl font-semibold">{node.name}</h1>
              <p className="truncate text-sm text-muted-foreground">{node.address}</p>
            </div>
          </div>
          {node.kind === "direct" && (
            <Button
              variant="destructive"
              className="w-full sm:w-auto"
              onClick={() => setUnlinkOpen(true)}
              disabled={unlinkPeer.isPending}
            >
              <Unplug className="h-4 w-4" />
              Desvincular
            </Button>
          )}
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="grid h-auto w-full grid-cols-5 sm:inline-grid sm:w-auto">
          <TabsTrigger value="overview">
            <LayoutGrid className="h-4 w-4" />
            Overview
          </TabsTrigger>
          <TabsTrigger value="services">
            <Globe2 className="h-4 w-4" />
            Serviços ({services.length})
          </TabsTrigger>
          <TabsTrigger value="neighbors">
            <Cable className="h-4 w-4" />
            Vizinhos ({neighbors.length})
          </TabsTrigger>
          <TabsTrigger value="routes">
            <Route className="h-4 w-4" />
            Rotas ({visibleRoutes.length})
          </TabsTrigger>
          <TabsTrigger value="ca">
            <ShieldCheck className="h-4 w-4" />
            Certificado
          </TabsTrigger>
        </TabsList>
        <TabsContent value="overview">
          <BindnetOverviewTab node={node} metrics={metrics} />
        </TabsContent>
        <TabsContent value="services">
          <BindnetServicesTab services={services} />
        </TabsContent>
        <TabsContent value="neighbors">
          <BindnetNeighborsTab neighbors={neighbors} />
        </TabsContent>
        <TabsContent value="routes">
          <BindnetRoutesTab
            routes={visibleRoutes}
            forgetPending={forgetRoute.isPending}
            onForget={(domain) => forgetRoute.mutate(domain)}
          />
        </TabsContent>
        <TabsContent value="ca">
          <BindnetCaSection node={node} />
        </TabsContent>
      </Tabs>
      <ConfirmDialog
        open={unlinkOpen}
        onOpenChange={setUnlinkOpen}
        title="Desvincular servidor"
        description={`Remover ${node.address} dos peers diretos e aplicar a configuração de DNS agora.`}
        confirmLabel="Desvincular"
        variant="destructive"
        pending={unlinkPeer.isPending}
        onConfirm={() => unlinkPeer.mutate()}
      />
    </div>
  );
}
