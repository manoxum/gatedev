import { CaCommonNameCard } from "@/components/settings/CaCommonNameCard";
import { NginxUiCredentialsCard } from "@/components/settings/NginxUiCredentialsCard";
import { useSettings } from "@/components/settings/useSettings";
import { usePageHeader } from "@/hooks/usePageHeader";

export function SettingsPage() {
  usePageHeader({
    title: "Configurações",
    description: "Ajustes do painel que antes viviam no arquivo .env do servidor.",
  });

  const { settings, save } = useSettings();

  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">
        Estes valores ficam guardados no banco de dados, não em arquivo de ambiente — editar aqui é o único caminho.
      </p>

      <CaCommonNameCard settings={settings} save={save} />
      <NginxUiCredentialsCard settings={settings} save={save} />
    </div>
  );
}
