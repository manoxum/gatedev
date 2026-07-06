import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { unlinkPeerAddress, useMeshData, type BindnetNode } from "@/lib/mesh";
import { AddBindnetFab } from "@/components/bindnets/AddBindnetFab";
import { BindnetList } from "@/components/bindnets/BindnetList";
import { usePageHeader } from "@/hooks/usePageHeader";

export function BindnetsPage() {
  usePageHeader({ title: "Servidores Bindnet", description: "Nós, serviços e rotas descobertos pela malha." });

  const queryClient = useQueryClient();
  const [unlinkTarget, setUnlinkTarget] = useState<BindnetNode | null>(null);
  const mesh = useMeshData();
  const data = mesh.data;
  const nodes = data?.nodes ?? [];

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
      <BindnetList nodes={nodes} data={data} isLoading={mesh.isLoading} onUnlink={setUnlinkTarget} />
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
