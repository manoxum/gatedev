import { Link } from "react-router-dom";
import { ChevronRight, Fingerprint, Globe2, Server, ServerCog, Unplug } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import {
  formatSeen,
  nodePath,
  nodeTone,
  nodeLabel,
  serviceRowsForNode,
  type BindnetNode,
  type MeshData,
} from "@/lib/mesh";
import { EmptyState } from "@/components/bindnets/EmptyState";

interface BindnetCardGridProps {
  nodes: BindnetNode[];
  data?: MeshData;
  isLoading: boolean;
  onUnlink: (node: BindnetNode) => void;
}

export function BindnetCardGrid({ nodes, data, isLoading, onUnlink }: BindnetCardGridProps) {
  return (
    <div className="grid gap-3 lg:grid-cols-3">
      {isLoading &&
        Array.from({ length: 3 }).map((_, index) => (
          <div key={index} className="h-36 animate-pulse rounded-lg border bg-muted/30" />
        ))}
      {!isLoading && nodes.length === 0 && <EmptyState label="Nenhum servidor encontrado." />}
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
                    onClick={() => onUnlink(node)}
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
  );
}
