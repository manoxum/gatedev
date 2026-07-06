import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";
import type { HotspotClient } from "@/components/hotspot/HotspotClientsCard";

interface UseHotspotMutationsOptions {
  onSaveSuccess: () => void;
  onRecoverSuccess: () => void;
}

export function useHotspotMutations({ onSaveSuccess, onRecoverSuccess }: UseHotspotMutationsOptions) {
  const queryClient = useQueryClient();

  // Salva e já aplica (reinicia o hotspot) numa única ação - separar em
  // dois passos só criava a falsa impressão de que "salvar" bastava, mas
  // a config nova só valia depois de "aplicar" mesmo assim.
  const saveAndApply = useMutation({
    mutationFn: async (data: ConfigForm) => {
      await api.patch("/hotspot/config", data);
      await api.post("/hotspot/apply");
    },
    onSuccess: () => {
      toast.success("Configuração salva e hotspot reiniciado com os novos valores.");
      queryClient.invalidateQueries({ queryKey: ["hotspot"] });
      onSaveSuccess();
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao salvar/aplicar"),
  });

  const start = useMutation({
    mutationFn: () => api.post("/hotspot/start"),
    onSuccess: () => {
      toast.success("Hotspot iniciado.");
      queryClient.invalidateQueries({ queryKey: ["hotspot"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao iniciar"),
  });

  const stop = useMutation({
    mutationFn: () => api.post("/hotspot/stop"),
    onSuccess: () => {
      toast.success("Hotspot parado.");
      queryClient.invalidateQueries({ queryKey: ["hotspot"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao parar"),
  });

  const recoverWifi = useMutation({
    mutationFn: () => api.post("/hotspot/recover-wifi"),
    onSuccess: () => {
      toast.success("Adaptador Wi-Fi recuperado.");
      queryClient.invalidateQueries({ queryKey: ["hotspot"] });
      onRecoverSuccess();
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao recuperar Wi-Fi"),
  });

  const block = useMutation({
    mutationFn: (mac: string) => api.post("/hotspot/blocklist", { mac }),
    onSuccess: () => {
      toast.success("Cliente bloqueado.");
      queryClient.invalidateQueries({ queryKey: ["hotspot", "clients"] });
      queryClient.invalidateQueries({ queryKey: ["hotspot", "blocklist"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao bloquear cliente"),
  });

  const unblock = useMutation({
    mutationFn: (mac: string) => api.del(`/hotspot/blocklist/${encodeURIComponent(mac)}`),
    onSuccess: () => {
      toast.success("Cliente desbloqueado.");
      queryClient.invalidateQueries({ queryKey: ["hotspot", "clients"] });
      queryClient.invalidateQueries({ queryKey: ["hotspot", "blocklist"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao desbloquear cliente"),
  });

  return { saveAndApply, start, stop, recoverWifi, block, unblock };
}

// useIdentifyDevice e useUpdateDeviceIdentity são independentes de
// useHotspotMutations (que exige callbacks da tela de configuração do
// hotspot) porque são usados a partir do modal "Identificar"
// (DeviceIdentifyModal.tsx) e da aba de visão geral do dispositivo
// (DeviceOverviewTab.tsx), que não passam por lá.

// useIdentifyDevice dispara a identificação automática (fabricante via
// MAC, fingerprint DHCP, heurística de SO) - "Buscar automaticamente"
// no modal.
export function useIdentifyDevice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (mac: string) => api.post<HotspotClient>(`/hotspot/clients/${encodeURIComponent(mac)}/identify`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["hotspot", "clients"] });
    },
    onError: (error) => toast.error(error instanceof ApiError ? error.message : "Falha ao identificar cliente"),
  });
}

// useUpdateDeviceIdentity grava edições manuais (alias/vendor/
// deviceName/osName) - campos omitidos do objeto (undefined) não são
// enviados no JSON e preservam o valor atual no backend (ver
// hotspotIdentityRequest em services/backend/hotspot_device_identity.go).
interface DeviceIdentityEdit {
  mac: string;
  alias?: string;
  vendor?: string;
  deviceName?: string;
  osName?: string;
}

export function useUpdateDeviceIdentity() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ mac, ...edit }: DeviceIdentityEdit) =>
      api.patch<HotspotClient>(`/hotspot/devices/${encodeURIComponent(mac)}/identity`, edit),
    onSuccess: () => {
      toast.success("Identificação salva.");
      queryClient.invalidateQueries({ queryKey: ["hotspot", "clients"] });
    },
    onError: (error) =>
      toast.error(
        error instanceof ApiError && error.status === 409 ? "Esse alias já está em uso." : "Falha ao salvar identificação",
      ),
  });
}
