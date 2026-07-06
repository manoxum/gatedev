import { cn } from "@/lib/utils";

interface SetupStepperProps {
  steps: string[];
  currentStep: number;
}

export function SetupStepper({ steps, currentStep }: SetupStepperProps) {
  return (
    <ol className="flex items-center gap-2 text-sm">
      {steps.map((label, index) => (
        <li key={label} className="flex items-center gap-2">
          <span
            className={cn(
              "flex h-6 w-6 items-center justify-center rounded-full border text-xs font-medium",
              index === currentStep && "border-primary bg-primary text-primary-foreground",
              index < currentStep && "border-primary/50 bg-primary/10 text-primary",
              index > currentStep && "border-muted-foreground/30 text-muted-foreground",
            )}
          >
            {index + 1}
          </span>
          <span className={cn(index === currentStep ? "font-medium" : "text-muted-foreground")}>{label}</span>
          {index < steps.length - 1 && <span className="mx-1 h-px w-6 bg-border" />}
        </li>
      ))}
    </ol>
  );
}
