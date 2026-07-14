import { useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { Eye, KeyRound, Wifi } from "lucide-react";

interface HotspotWifiQrProps {
  ssid: string;
  password: string;
}

function wifiQrValue(ssid: string, password: string) {
  const escape = (value: string) => value.replace(/([\\;,:"])/g, "\\$1");
  return `WIFI:T:WPA;S:${escape(ssid)};P:${escape(password)};;`;
}

// Cartão do QR de conexão Wi-Fi: moldura em degradê na cor da marca em volta
// de um miolo branco (o branco é o que garante boa leitura pela câmera em
// qualquer tema, então nunca deve seguir o fundo escuro do dark mode).
// Clicável: gira em 3D (CSS "flip card", sem lib extra) revelando o SSID e
// a senha em texto puro no verso - útil quando a câmera não está à mão
// (ex.: digitar a senha manualmente em outro aparelho).
export function HotspotWifiQr({ ssid, password }: HotspotWifiQrProps) {
  const [flipped, setFlipped] = useState(false);

  const toggle = () => setFlipped((current) => !current);
  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      toggle();
    }
  };

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={toggle}
      onKeyDown={handleKeyDown}
      aria-label={flipped ? "Mostrar QR code para conectar" : "Mostrar nome da rede e senha"}
      className="h-full shrink-0 cursor-pointer select-none rounded-2xl outline-none focus-visible:ring-2 focus-visible:ring-ring"
      style={{ perspective: 1000 }}
    >
      <div
        className="relative h-full w-full rounded-2xl bg-gradient-to-br from-primary/30 via-primary/10 to-transparent p-[3px] shadow-elevated transition-transform duration-500 hover:scale-[1.02]"
        style={{ transformStyle: "preserve-3d", transform: flipped ? "rotateY(180deg)" : "rotateY(0deg)" }}
      >
        <div
          className="flex h-full min-w-[190px] flex-col items-center justify-center gap-4 rounded-[calc(1rem-1px)] bg-white px-6 py-5"
          style={{ backfaceVisibility: "hidden" }}
        >
          <div className="relative">
            <QRCodeSVG value={wifiQrValue(ssid, password)} size={176} fgColor="#065f46" bgColor="#ffffff" level="M" />
            <div className="absolute inset-0 flex items-center justify-center">
              <div className="flex h-10 w-10 items-center justify-center rounded-full border-2 border-white bg-emerald-600 shadow-sm">
                <Wifi className="h-5 w-5 text-white" />
              </div>
            </div>
          </div>
          <p className="text-xs font-medium text-emerald-700">Escaneie para conectar</p>
          <p className="text-[10px] text-emerald-700/60">toque para ver a senha</p>
        </div>

        <div
          className="absolute inset-0 flex h-full min-w-[190px] flex-col items-center justify-center gap-4 rounded-[calc(1rem-1px)] bg-white px-6 py-5"
          style={{ backfaceVisibility: "hidden", transform: "rotateY(180deg)" }}
        >
          <div className="flex h-10 w-10 items-center justify-center rounded-full border-2 border-white bg-emerald-600 shadow-sm">
            <Wifi className="h-5 w-5 text-white" />
          </div>
          <div className="flex w-full flex-col items-center gap-1">
            <span className="text-[10px] uppercase tracking-wide text-emerald-700/60">Rede</span>
            <p className="max-w-full truncate text-sm font-semibold text-emerald-900">{ssid}</p>
          </div>
          <div className="flex w-full flex-col items-center gap-1">
            <span className="flex items-center gap-1 text-[10px] uppercase tracking-wide text-emerald-700/60">
              <KeyRound className="h-3 w-3" /> Senha
            </span>
            <p className="max-w-full truncate font-mono text-sm font-semibold text-emerald-900">{password}</p>
          </div>
          <p className="flex items-center gap-1 text-[10px] text-emerald-700/60">
            <Eye className="h-3 w-3" /> toque para ver o QR
          </p>
        </div>
      </div>
    </div>
  );
}
