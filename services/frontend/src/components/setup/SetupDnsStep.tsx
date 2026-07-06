import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription, CardFooter } from "@/components/ui/card";
import { DnsLocalTldsCard } from "@/components/dns/DnsLocalTldsCard";
import { useDnsQueries } from "@/components/dns/useDnsQueries";
import { useDnsMutations } from "@/components/dns/useDnsMutations";

interface SetupDnsStepProps {
  initialTlds?: string[];
  onNext: (tlds: string[]) => void;
  onSkip: () => void;
  onBack: () => void;
}

// Só coleta os TLDs - nada é salvo/aplicado aqui, isso só acontece no
// último passo (SetupStatusStep), de uma vez só.
export function SetupDnsStep({ initialTlds, onNext, onSkip, onBack }: SetupDnsStepProps) {
  const { config } = useDnsQueries();
  const mutations = useDnsMutations();
  const [tlds, setTlds] = useState<string[]>(initialTlds ?? []);

  const effectiveConfig = initialTlds ? { ...config.data, DNS_LOCAL_TLDS: initialTlds.join(",") } : config.data;

  return (
    <Card>
      <CardHeader>
        <CardTitle>DNS local</CardTitle>
        <CardDescription>
          Defina os TLDs locais (ex.: .local). A configuração só é salva e aplicada ao final do assistente.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <DnsLocalTldsCard config={effectiveConfig} mutations={mutations} showActions={false} onChange={setTlds} />
      </CardContent>
      <CardFooter className="justify-between gap-2 border-t pt-4">
        <Button variant="outline" onClick={onBack}>
          Voltar
        </Button>
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onSkip}>
            Pular por agora
          </Button>
          <Button onClick={() => onNext(tlds)}>Continuar</Button>
        </div>
      </CardFooter>
    </Card>
  );
}
