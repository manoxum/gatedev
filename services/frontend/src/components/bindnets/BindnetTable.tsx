import { Link } from "react-router-dom";
import { Server, ServerCog, Unplug } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
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

interface BindnetTableProps {
  nodes: BindnetNode[];
  data?: MeshData;
  emptyMessage: string;
  onUnlink: (node: BindnetNode) => void;
}

export function BindnetTable({ nodes, data, emptyMessage, onUnlink }: BindnetTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Servidor</TableHead>
          <TableHead>Endereço</TableHead>
          <TableHead>Tipo</TableHead>
          <TableHead>Domínios</TableHead>
          <TableHead>Serviços</TableHead>
          <TableHead>Visto por último</TableHead>
          <TableHead className="text-right">Ações</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {nodes.map((node) => {
          const services = data ? serviceRowsForNode(node, data).length : 0;
          const domains = node.domains?.slice(0, 2).join(", ") || "sem domínio anunciado";
          return (
            <TableRow key={node.id}>
              <TableCell>
                <Link to={nodePath(node.id)} className="flex items-center gap-2 font-medium hover:underline">
                  {node.kind === "local" ? <ServerCog className="h-4 w-4" /> : <Server className="h-4 w-4" />}
                  {node.name}
                </Link>
              </TableCell>
              <TableCell className="text-muted-foreground">{node.address}</TableCell>
              <TableCell>
                <Badge variant="outline" className={cn("border", nodeTone(node.kind))}>
                  {nodeLabel(node.kind)}
                </Badge>
              </TableCell>
              <TableCell className="max-w-48 truncate text-muted-foreground">{domains}</TableCell>
              <TableCell>{services}</TableCell>
              <TableCell className="text-muted-foreground">{formatSeen(node.lastSeenAt)}</TableCell>
              <TableCell className="text-right">
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
              </TableCell>
            </TableRow>
          );
        })}
        {nodes.length === 0 && (
          <TableRow>
            <TableCell colSpan={7} className="text-center text-muted-foreground">
              {emptyMessage}
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  );
}
