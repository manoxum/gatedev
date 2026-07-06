import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import type { useDnsMutations } from "@/components/dns/useDnsMutations";

interface DnsTestResolutionCardProps {
  mutations: ReturnType<typeof useDnsMutations>;
}

export function DnsTestResolutionCard({ mutations }: DnsTestResolutionCardProps) {
  const { test } = mutations;
  const [hostname, setHostname] = useState("");

  return (
    <Card>
      <CardHeader>
        <CardTitle>Testar resolução</CardTitle>
        <CardDescription>Confirme se um hostname resolve como esperado.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex gap-2">
          <Label htmlFor="testHostname" className="sr-only">
            Hostname
          </Label>
          <Input
            id="testHostname"
            placeholder="ex.: painel.local"
            value={hostname}
            onChange={(e) => setHostname(e.target.value)}
          />
          <Button onClick={() => test.mutate(hostname)} disabled={!hostname || test.isPending}>
            Testar
          </Button>
        </div>
        {test.data && (
          <p className="text-sm">
            {test.data.error ? `Erro: ${test.data.error}` : `Resolvido para: ${test.data.addresses?.join(", ")}`}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
