import { useTheme } from "@/hooks/useTheme";

// ApexCharts precisa de cores concretas (hex/hsl resolvido) - nao
// aceita "hsl(var(--primary))" como os SVGs desenhados a mao deste
// projeto aceitavam (o calculo de gradiente/tooltip da lib e feito em
// JS, nao repassado puro pro CSS do navegador). Os valores abaixo sao
// uma copia literal dos tokens de src/index.css (claro/escuro) - se o
// tema mudar la, atualize aqui tambem.
const PALETTE = {
  light: {
    primary: "hsl(160, 84%, 30%)",
    primaryMuted: "hsl(160, 84%, 30%, 0.35)",
    // secondary (upload): azul, deliberadamente longe do verde
    // (download/primary) e do vermelho (destructive/limite) na roda de
    // cores - as duas series do grafico de velocidade precisam ser
    // discriminaveis por cor, nao so por opacidade.
    secondary: "hsl(217, 91%, 55%)",
    secondaryMuted: "hsl(217, 91%, 55%, 0.35)",
    destructive: "hsl(0, 84.2%, 60.2%)",
    foreground: "hsl(222, 20%, 12%)",
    mutedForeground: "hsl(215, 15%, 40%)",
    border: "hsl(220, 13%, 85%)",
  },
  dark: {
    primary: "hsl(160, 70%, 42%)",
    primaryMuted: "hsl(160, 70%, 42%, 0.35)",
    secondary: "hsl(217, 91%, 68%)",
    secondaryMuted: "hsl(217, 91%, 68%, 0.35)",
    destructive: "hsl(0, 62.8%, 40%)",
    foreground: "hsl(210, 30%, 96%)",
    mutedForeground: "hsl(216, 20%, 68%)",
    border: "hsl(222, 25%, 20%)",
  },
} as const;

// useApexChartColors devolve a paleta resolvida pro tema ativo agora -
// reativo (o componente que chamar isso re-renderiza sozinho quando o
// usuario alterna claro/escuro, via useTheme/ThemeContext).
export function useApexChartColors() {
  const { theme } = useTheme();
  return PALETTE[theme];
}
