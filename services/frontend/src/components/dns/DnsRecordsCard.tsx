import { useState } from "react";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import type { DnsRecord } from "@/components/dns/useDnsQueries";
import type { useDnsMutations } from "@/components/dns/useDnsMutations";

interface DnsRecordsCardProps {
  records: DnsRecord[];
  mutations: ReturnType<typeof useDnsMutations>;
}

export function DnsRecordsCard({ records, mutations }: DnsRecordsCardProps) {
  const { addRecord, removeRecord, clearRecords } = mutations;
  const [newHostname, setNewHostname] = useState("");
  const [confirmClearOpen, setConfirmClearOpen] = useState(false);

  function submitAdd() {
    const hostname = newHostname.trim().toLowerCase();
    if (!hostname) return;
    addRecord.mutate(hostname, { onSuccess: () => setNewHostname("") });
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-4">
          <div>
            <CardTitle>DNS já resolvido ({records.length})</CardTitle>
            <CardDescription>
              Hostnames que já ganharam um IP de loopback fixo (view host do split-horizon). Adicione manualmente
              para reservar o IP antes da primeira consulta, ou remova entradas que não são mais usadas.
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            disabled={records.length === 0}
            onClick={() => setConfirmClearOpen(true)}
          >
            <Trash2 className="h-4 w-4" />
            Limpar tudo
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex gap-2">
          <Input
            placeholder="ex.: painel.local"
            value={newHostname}
            onChange={(e) => setNewHostname(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), submitAdd())}
          />
          <Button type="button" onClick={submitAdd} disabled={!newHostname.trim() || addRecord.isPending}>
            Adicionar
          </Button>
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Hostname</TableHead>
              <TableHead>Endereço</TableHead>
              <TableHead>Criado em</TableHead>
              <TableHead className="text-right">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {records.map((record) => (
              <TableRow key={record.hostname}>
                <TableCell className="font-mono text-xs">{record.hostname}</TableCell>
                <TableCell className="font-mono text-xs">{record.address}</TableCell>
                <TableCell>{new Date(record.createdAt).toLocaleString()}</TableCell>
                <TableCell>
                  <div className="flex justify-end">
                    <Button
                      variant="secondary"
                      size="sm"
                      disabled={removeRecord.isPending && removeRecord.variables === record.hostname}
                      onClick={() => removeRecord.mutate(record.hostname)}
                    >
                      <Trash2 className="h-4 w-4" />
                      Remover
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {records.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="text-center text-muted-foreground">
                  Nenhum hostname resolvido ainda.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>

      <ConfirmDialog
        open={confirmClearOpen}
        onOpenChange={setConfirmClearOpen}
        title="Limpar todos os registros?"
        description="Todos os hostnames resolvidos perdem o IP de loopback reservado. Eles ganham um novo IP na próxima consulta."
        confirmLabel="Limpar tudo"
        variant="destructive"
        pending={clearRecords.isPending}
        onConfirm={() => clearRecords.mutate(undefined, { onSuccess: () => setConfirmClearOpen(false) })}
      />
    </Card>
  );
}
