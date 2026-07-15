import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface LogsPanelProps {
  title: string;
  path: string;
  // onClear e opcional - so a tela de hotspot (ate agora) grava um
  // corte de tempo no backend (POST /api/hotspot/logs/clear) para
  // "esquecer" os logs antigos; quando ausente, o botao "Limpar
  // logs" nem aparece (ex.: tela de DNS).
  onClear?: () => Promise<void> | void;
}

// LogsPanel le o stream de texto puro que o backend repassa do worker
// (docker logs -f) e vai anexando ao painel, com auto-scroll.
export function LogsPanel({ title, path, onClear }: LogsPanelProps) {
  const [lines, setLines] = useState("");
  // Comeca acompanhando ja: o componente so monta quando a aba/tela de
  // logs correspondente e aberta (Radix TabsContent so monta a aba
  // ativa), entao "entrar na aba" e "montar o LogsPanel" e a mesma
  // coisa - nao faz sentido exigir um clique extra em "Acompanhar
  // logs" toda vez.
  const [following, setFollowing] = useState(true);
  const [clearing, setClearing] = useState(false);
  // Incrementado a cada "Limpar logs" bem sucedido so para forcar o
  // efeito abaixo a reabrir o stream (se estiver acompanhando) ja a
  // partir do novo corte de tempo gravado no backend.
  const [resetToken, setResetToken] = useState(0);
  const preRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    if (!following) return;
    const controller = new AbortController();
    setLines("");

    fetch(`/api${path}?tail=200&follow=true`, { credentials: "include", signal: controller.signal })
      .then(async (response) => {
        const reader = response.body?.getReader();
        if (!reader) return;
        const decoder = new TextDecoder();
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          setLines((current) => current + decoder.decode(value, { stream: true }));
        }
      })
      .catch(() => {
        // requisição abortada ao pausar/desmontar - comportamento esperado
      });

    return () => controller.abort();
  }, [following, path, resetToken]);

  useEffect(() => {
    preRef.current?.scrollTo({ top: preRef.current.scrollHeight });
  }, [lines]);

  const handleClear = async () => {
    if (!onClear) return;
    setClearing(true);
    try {
      await onClear();
      setLines("");
      setResetToken((v) => v + 1);
    } catch {
      // erro ja mostrado via toast por quem passou onClear
    } finally {
      setClearing(false);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <CardTitle className="text-base">{title}</CardTitle>
        <div className="flex items-center gap-2">
          {onClear && (
            <Button size="sm" variant="outline" onClick={handleClear} disabled={clearing}>
              Limpar logs
            </Button>
          )}
          <Button size="sm" variant={following ? "destructive" : "outline"} onClick={() => setFollowing((v) => !v)}>
            {following ? "Parar" : "Acompanhar logs"}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <pre ref={preRef} className="h-64 overflow-auto rounded-md bg-black p-3 text-xs text-green-400">
          {lines || "Clique em 'Acompanhar logs' para ver a saída em tempo real."}
        </pre>
      </CardContent>
    </Card>
  );
}
