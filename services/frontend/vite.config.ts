import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import path from "path";

export default defineConfig(() => {
  const apiTarget = process.env.VITE_DEV_API_TARGET ?? "http://127.0.0.1:8090";

  return {
    server: {
      host: "::",
      port: 9090,
      // Nomes servidos via nginx-ui (proxy reverso na frente do dev
      // server, ver /etc/nginx/sites-available/{client,admin}.bindnet.local
      // no container nginx-ui) - sem isso o Vite bloqueia com 403
      // "Blocked request" por causa da protecao contra DNS rebinding,
      // que rejeita qualquer Host header fora da allowlist.
      allowedHosts: ["client.bindnet.local", "admin.bindnet.local"],
      proxy: {
        "/api": {
          target: apiTarget,
          changeOrigin: true,
          // xfwd: preenche X-Forwarded-For com o IP real do dispositivo
          // que conectou no dev server - sem isso o proxy do Vite nao
          // manda esse cabecalho, e o backend cai pro RemoteAddr da
          // conexao vinda de "target" (127.0.0.1:8090 -> porta
          // publicada do backend numa rede bridge), que o
          // docker-proxy/iptables reescreve para o gateway da rede -
          // quebra resolvePortalMAC (services/backend/hotspot_portal.go)
          // em dev do mesmo jeito que "network_mode: host" evita em
          // producao (ver docker-compose.services.yml).
          xfwd: true,
        },
      },
    },
    plugins: [react()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    build: {
      // Facilita mapear stack traces do bundle minificado de volta pro
      // codigo-fonte (ex.: erros reportados so pelo console do navegador,
      // como o que motivou o ErrorBoundary em src/components/ErrorBoundary.tsx).
      sourcemap: true,
    },
  };
});
