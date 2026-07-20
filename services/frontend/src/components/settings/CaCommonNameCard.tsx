import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import type { useSettings } from "@/components/settings/useSettings";

interface CaCommonNameCardProps {
  settings: ReturnType<typeof useSettings>["settings"];
  save: ReturnType<typeof useSettings>["save"];
}

export function CaCommonNameCard({ settings, save }: CaCommonNameCardProps) {
  const [commonName, setCommonName] = useState("");

  useEffect(() => {
    if (settings.data) setCommonName(settings.data.caCommonName);
  }, [settings.data]);

  const generated = settings.data?.caGenerated ?? false;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Autoridade certificadora (CA) local</CardTitle>
        <CardDescription>
          Nome comum (CN) que aparece como emissor nos certificados assinados pelo painel.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {generated && (
          <div className="space-y-2">
            <Label>CA atual (não muda)</Label>
            <Input value={settings.data?.caCurrentCommonName ?? ""} readOnly disabled />
            <p className="text-sm text-muted-foreground">
              Já existe uma CA gerada. Trocar o nome abaixo <strong>não</strong> renomeia nem reemite esta CA — os
              dispositivos que já confiam nela continuam funcionando. O novo nome só valeria para uma CA gerada do
              zero.
            </p>
          </div>
        )}

        <div className="space-y-2">
          <Label htmlFor="caCommonName">Nome comum para uma CA nova</Label>
          <Input
            id="caCommonName"
            placeholder="ex.: Bindnet Local Trust CA"
            value={commonName}
            onChange={(e) => setCommonName(e.target.value)}
          />
          {!generated && (
            <p className="text-sm text-muted-foreground">
              Nenhuma CA foi gerada ainda — este é o nome que ela vai usar quando for criada.
            </p>
          )}
        </div>

        <Button
          onClick={() => save.mutate({ caCommonName: commonName })}
          disabled={save.isPending || settings.isLoading}
        >
          {save.isPending ? "Salvando..." : "Salvar"}
        </Button>
      </CardContent>
    </Card>
  );
}
