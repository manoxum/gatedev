import { useState } from "react";
import { ViewToggle, type ViewMode } from "@/components/ui/view-toggle";
import { BindnetCardGrid } from "@/components/bindnets/BindnetCard";
import { BindnetTable } from "@/components/bindnets/BindnetTable";
import type { BindnetNode, MeshData } from "@/lib/mesh";

interface BindnetListProps {
  nodes: BindnetNode[];
  data?: MeshData;
  isLoading: boolean;
  onUnlink: (node: BindnetNode) => void;
}

// Alterna entre visão em cards e em tabela para a listagem de servidores Bindnet.
export function BindnetList({ nodes, data, isLoading, onUnlink }: BindnetListProps) {
  const [view, setView] = useState<ViewMode>("grid");

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <ViewToggle value={view} onChange={setView} />
      </div>
      {view === "grid" ? (
        <BindnetCardGrid nodes={nodes} data={data} isLoading={isLoading} onUnlink={onUnlink} />
      ) : isLoading ? (
        <div className="h-32 animate-pulse rounded-lg border bg-muted/30" />
      ) : (
        <BindnetTable nodes={nodes} data={data} emptyMessage="Nenhum servidor encontrado." onUnlink={onUnlink} />
      )}
    </div>
  );
}
