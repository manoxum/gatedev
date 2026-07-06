import { Navigate, useLocation } from "react-router-dom";
import { useSession } from "@/hooks/useAuth";
import { useSetupStatus } from "@/hooks/useSetupStatus";

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { data, isLoading, isError } = useSession();
  const location = useLocation();
  const setupStatus = useSetupStatus({ enabled: !!data });

  if (isLoading) {
    return <div className="flex min-h-screen items-center justify-center text-muted-foreground">Carregando...</div>;
  }
  if (isError || !data) {
    return <Navigate to="/login" replace />;
  }
  if (setupStatus.data && !setupStatus.data.hotspotConfigured && location.pathname !== "/setup") {
    return <Navigate to="/setup" replace />;
  }
  return <>{children}</>;
}
