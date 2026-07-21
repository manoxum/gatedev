import { ArrowLeftRight, ArrowRight, Pencil, Power, Trash2, Users } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { isWithinProfileRule } from "@/components/hotspot/hotspot-isolation-schema";
import type { HotspotCommRule } from "@/components/hotspot/hotspot-isolation-types";

interface HotspotIsolationRulesTableProps {
  rules: HotspotCommRule[];
  // Nome legível de uma ponta, sem prefixo de tipo (a tabela adiciona
  // "Perfil"/"Cliente" conforme a coluna).
  endpointName: (kind: string, ref: string | null) => string;
  pendingId?: string;
  onEdit: (rule: HotspotCommRule) => void;
  onToggleEnabled: (rule: HotspotCommRule) => void;
  onDelete: (rule: HotspotCommRule) => void;
}

const KIND_LABEL: Record<string, string> = { profile: "Perfil", device: "Cliente", any: "" };

function endpointText(endpointName: HotspotIsolationRulesTableProps["endpointName"], kind: string, ref: string | null) {
  const name = endpointName(kind, ref);
  const prefix = KIND_LABEL[kind];
  return prefix ? `${prefix} · ${name}` : name;
}

// Resumo L4 legível: "" para tráfego irrestrito, senão protocolo (+
// portas). Ex.: "TCP 80,443", "ICMP".
function l4Summary(rule: HotspotCommRule): string {
  if (rule.protocol === "any") return "";
  const proto = rule.protocol.toUpperCase();
  return rule.dstPorts ? `${proto} ${rule.dstPorts}` : proto;
}

// Tabela das regras de comunicação da aba Isolamento - só apresentação.
// Regra "dentro de um perfil" (ambas as pontas no mesmo perfil) é
// mostrada como uma linha própria; as demais mostram origem, sentido e
// destino. Colunas secundárias somem no celular e as ações viram
// só-ícone (padrão de responsividade do CLAUDE.md).
export function HotspotIsolationRulesTable({
  rules,
  endpointName,
  pendingId,
  onEdit,
  onToggleEnabled,
  onDelete,
}: HotspotIsolationRulesTableProps) {
  if (rules.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        Nenhuma regra criada. Com o isolamento ativo, os clientes só comunicam conforme as regras adicionadas aqui —
        use "Nova regra" para liberar a comunicação dentro de um perfil ou entre uma origem e um destino.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Origem</TableHead>
          <TableHead className="w-10 text-center" aria-label="Sentido" />
          <TableHead>Destino</TableHead>
          <TableHead>Ação</TableHead>
          <TableHead className="hidden sm:table-cell">Estado</TableHead>
          <TableHead className="hidden md:table-cell">Observação</TableHead>
          <TableHead className="text-right">Ações</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rules.map((rule) => {
          const within = isWithinProfileRule(rule);
          return (
            <TableRow key={rule.id} className={rule.enabled ? undefined : "opacity-50"}>
              {within ? (
                <TableCell className="font-medium" colSpan={3}>
                  <span className="inline-flex items-center gap-1.5">
                    <Users className="h-4 w-4 text-muted-foreground" />
                    Dentro do perfil · {endpointName("profile", rule.sourceRef)}
                  </span>
                </TableCell>
              ) : (
                <>
                  <TableCell className="font-medium">{endpointText(endpointName, rule.sourceKind, rule.sourceRef)}</TableCell>
                  <TableCell className="text-center">
                    {rule.direction === "both" ? (
                      <ArrowLeftRight className="inline h-4 w-4 text-muted-foreground" aria-label="ambos os sentidos" />
                    ) : (
                      <ArrowRight className="inline h-4 w-4 text-muted-foreground" aria-label="só origem para destino" />
                    )}
                  </TableCell>
                  <TableCell>{endpointText(endpointName, rule.targetKind, rule.targetRef)}</TableCell>
                </>
              )}
              <TableCell>
                <div className="flex flex-col items-start gap-1">
                  <Badge variant={rule.action === "allow" ? "success" : "destructive"}>
                    {rule.action === "allow" ? "Permitir" : "Bloquear"}
                  </Badge>
                  {l4Summary(rule) && (
                    <span className="text-[11px] font-medium text-muted-foreground">{l4Summary(rule)}</span>
                  )}
                </div>
              </TableCell>
              <TableCell className="hidden sm:table-cell">
                <Badge variant={rule.enabled ? "secondary" : "outline"}>{rule.enabled ? "Ativa" : "Inativa"}</Badge>
              </TableCell>
              <TableCell className="hidden max-w-48 truncate text-muted-foreground md:table-cell">
                {rule.note ?? ""}
              </TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Editar regra"
                    title="Editar regra"
                    disabled={pendingId === rule.id}
                    onClick={() => onEdit(rule)}
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={rule.enabled ? "Desativar regra" : "Ativar regra"}
                    title={rule.enabled ? "Desativar regra" : "Ativar regra"}
                    disabled={pendingId === rule.id}
                    onClick={() => onToggleEnabled(rule)}
                  >
                    <Power className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Remover regra"
                    title="Remover regra"
                    disabled={pendingId === rule.id}
                    onClick={() => onDelete(rule)}
                  >
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
