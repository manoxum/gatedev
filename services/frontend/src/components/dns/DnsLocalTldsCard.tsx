import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import type { useDnsMutations } from "@/components/dns/useDnsMutations";

interface DnsLocalTldsCardProps {
  config: Record<string, string> | undefined;
  mutations: ReturnType<typeof useDnsMutations>;
  // false esconde os botões Salvar/Aplicar e reporta a lista atual via
  // onChange - usado pelo assistente de configuração inicial, que só
  // salva/aplica tudo no último passo.
  showActions?: boolean;
  onChange?: (tlds: string[]) => void;
}

export function DnsLocalTldsCard({ config, mutations, showActions = true, onChange }: DnsLocalTldsCardProps) {
  const { saveTlds } = mutations;
  const [tlds, setTlds] = useState<string[]>([]);
  const [newTld, setNewTld] = useState("");

  useEffect(() => {
    if (config?.DNS_LOCAL_TLDS) {
      setTlds(config.DNS_LOCAL_TLDS.split(",").map((t) => t.trim()).filter(Boolean));
    }
  }, [config]);

  useEffect(() => {
    onChange?.(tlds);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tlds]);

  function addTld() {
    const value = newTld.trim().toLowerCase();
    if (!value || tlds.includes(value)) return;
    setTlds((current) => [...current, value]);
    setNewTld("");
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>TLDs locais</CardTitle>
        <CardDescription>
          Domínios como *.local respondem com o IP do hotspot; qualquer outro domínio é encaminhado normalmente.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap gap-2">
          {tlds.map((tld) => (
            <Badge key={tld} variant="secondary" className="gap-1">
              {tld}
              <button onClick={() => setTlds((current) => current.filter((t) => t !== tld))}>
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
        <div className="flex gap-2">
          <Input
            placeholder="ex.: local"
            value={newTld}
            onChange={(e) => setNewTld(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), addTld())}
          />
          <Button type="button" variant="outline" onClick={addTld}>
            Adicionar
          </Button>
        </div>
        {showActions && (
          <div className="flex gap-2">
            <Button onClick={() => saveTlds.mutate(tlds)} disabled={saveTlds.isPending || tlds.length === 0}>
              {saveTlds.isPending ? "Salvando e aplicando..." : "Salvar e aplicar"}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
