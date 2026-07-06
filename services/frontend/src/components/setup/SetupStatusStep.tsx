import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardFooter } from "@/components/ui/card";
import { useSetupStatus } from "@/hooks/useSetupStatus";
import { useFinishSetup, setupErrorMessage } from "@/components/setup/useSetupMutations";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";

interface SetupStatusStepProps {
  hotspotData?: ConfigForm;
  dnsTlds?: string[];
  onDone: () => void;
  onBack: () => void;
}

const serviceLabels: Record<string, string> = {
  postgres: "Postgres",
  mongo: "Mongo",
  minio: "MinIO",
};

export function SetupStatusStep({ hotspotData, dnsTlds, onDone, onBack }: SetupStatusStepProps) {
  const { data, isLoading } = useSetupStatus();
  const finishSetup = useFinishSetup();

  async function handleFinish() {
    try {
      await finishSetup.mutateAsync({ hotspot: hotspotData, dnsTlds });
      toast.success("Configuração salva e aplicada com sucesso.");
      onDone();
    } catch (error) {
      toast.error(setupErrorMessage(error, "Falha ao salvar/aplicar a configuração"));
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Status dos serviços</CardTitle>
        <CardDescription>Conectividade dos serviços já configurados via .env.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        {isLoading && <p className="text-sm text-muted-foreground">Verificando...</p>}
        {data &&
          Object.entries(data.services).map(([key, status]) => (
            <div key={key} className="flex items-center justify-between rounded border p-3">
              <span className="text-sm font-medium">{serviceLabels[key] ?? key}</span>
              <Badge variant={status.reachable ? "success" : "destructive"}>
                {status.reachable ? "conectado" : "indisponível"}
              </Badge>
            </div>
          ))}
      </CardContent>
      <CardFooter className="justify-between border-t pt-4">
        <Button variant="outline" onClick={onBack} disabled={finishSetup.isPending}>
          Voltar
        </Button>
        <Button onClick={handleFinish} disabled={finishSetup.isPending}>
          {finishSetup.isPending ? "Aplicando..." : "Concluir e aplicar configurações"}
        </Button>
      </CardFooter>
    </Card>
  );
}
