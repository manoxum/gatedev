import { Button } from "@/components/ui/button";

// Fallback do ErrorBoundary de topo (main.tsx) - so aparece se algum
// erro de render/commit escapar de todos os boundaries mais internos
// (ex.: SpeedGauge). Recarregar a pagina reseta o estado do React.
export function AppCrashFallback() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 p-6 text-center">
      <h1 className="text-lg font-semibold text-foreground">Algo deu errado</h1>
      <p className="max-w-sm text-sm text-muted-foreground">
        Ocorreu um erro inesperado na interface. Recarregue a página para continuar.
      </p>
      <Button onClick={() => window.location.reload()}>Recarregar</Button>
    </div>
  );
}
