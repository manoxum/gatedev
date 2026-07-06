import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { SetupStepper } from "@/components/setup/SetupStepper";
import { SetupHotspotStep } from "@/components/setup/SetupHotspotStep";
import { SetupDnsStep } from "@/components/setup/SetupDnsStep";
import { SetupStatusStep } from "@/components/setup/SetupStatusStep";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";

const steps = ["Hotspot", "DNS", "Status"];

export function SetupPage() {
  const [step, setStep] = useState(0);
  const [hotspotData, setHotspotData] = useState<ConfigForm | undefined>();
  const [dnsTlds, setDnsTlds] = useState<string[] | undefined>();
  const navigate = useNavigate();

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
      <div className="w-full max-w-2xl space-y-6">
        <div className="space-y-1 text-center">
          <h1 className="text-2xl font-semibold">Configuração inicial do bindnet</h1>
          <p className="text-sm text-muted-foreground">
            Configure o hotspot e o DNS - tudo é salvo e aplicado só no último passo.
          </p>
        </div>
        <SetupStepper steps={steps} currentStep={step} />
        {step === 0 && (
          <SetupHotspotStep
            initialData={hotspotData}
            onNext={(data) => {
              setHotspotData(data);
              setStep(1);
            }}
            onSkip={() => setStep(1)}
          />
        )}
        {step === 1 && (
          <SetupDnsStep
            initialTlds={dnsTlds}
            onNext={(tlds) => {
              setDnsTlds(tlds);
              setStep(2);
            }}
            onSkip={() => setStep(2)}
            onBack={() => setStep(0)}
          />
        )}
        {step === 2 && (
          <SetupStatusStep
            hotspotData={hotspotData}
            dnsTlds={dnsTlds}
            onDone={() => navigate("/", { replace: true })}
            onBack={() => setStep(1)}
          />
        )}
      </div>
    </div>
  );
}
