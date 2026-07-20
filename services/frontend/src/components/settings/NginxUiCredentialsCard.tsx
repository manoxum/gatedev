import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import type { useSettings } from "@/components/settings/useSettings";

interface NginxUiCredentialsCardProps {
  settings: ReturnType<typeof useSettings>["settings"];
  save: ReturnType<typeof useSettings>["save"];
}

export function NginxUiCredentialsCard({ settings, save }: NginxUiCredentialsCardProps) {
  const [username, setUsername] = useState("");
  // A senha nunca volta do backend: o campo começa vazio e só é enviado
  // quando o operador digita algo (ou pede para limpar explicitamente).
  const [password, setPassword] = useState("");

  useEffect(() => {
    if (settings.data) setUsername(settings.data.nginxUiUsername);
  }, [settings.data]);

  const configured = settings.data?.nginxUiConfigured ?? false;

  function submit() {
    save.mutate({
      nginxUiUsername: username,
      ...(password !== "" ? { nginxUiPassword: password } : {}),
    });
    setPassword("");
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          Credenciais do nginx-ui
          <Badge variant={configured ? "secondary" : "outline"}>
            {configured ? "Configuradas" : "Não configuradas"}
          </Badge>
        </CardTitle>
        <CardDescription>
          Usuário do próprio nginx-ui (não tem relação com o login deste painel), usado só para cadastrar o
          certificado nele automaticamente após a emissão. Sem credenciais a emissão continua funcionando — o
          certificado é importado direto no nginx-ui, sem passar pela API dele.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="nginxUiUsername">Usuário</Label>
            <Input
              id="nginxUiUsername"
              autoComplete="off"
              placeholder="ex.: admin"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="nginxUiPassword">Senha</Label>
            <Input
              id="nginxUiPassword"
              type="password"
              autoComplete="new-password"
              placeholder={configured ? "Inalterada — digite para trocar" : "Sem senha configurada"}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
        </div>

        <div className="flex flex-col gap-2 sm:flex-row">
          <Button onClick={submit} disabled={save.isPending || settings.isLoading}>
            {save.isPending ? "Salvando..." : "Salvar"}
          </Button>
          {configured && (
            <Button
              variant="outline"
              onClick={() => {
                setPassword("");
                save.mutate({ nginxUiUsername: "", nginxUiPassword: "" });
              }}
              disabled={save.isPending}
            >
              Remover credenciais
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
