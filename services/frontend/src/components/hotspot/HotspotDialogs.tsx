import type { FieldErrors, UseFormHandleSubmit, UseFormRegister } from "react-hook-form";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { HotspotConfigForm } from "@/components/hotspot/HotspotConfigForm";
import type { ConfigForm } from "@/components/hotspot/hotspot-schema";
import type { NetworkInterface } from "@/components/hotspot/useHotspotQueries";

interface HotspotDialogsProps {
  configOpen: boolean;
  onConfigOpenChange: (open: boolean) => void;
  register: UseFormRegister<ConfigForm>;
  errors: FieldErrors<ConfigForm>;
  handleSubmit: UseFormHandleSubmit<ConfigForm>;
  onSave: (data: ConfigForm) => void;
  isDirty: boolean;
  savePending: boolean;
  showPassword: boolean;
  onToggleShowPassword: () => void;
  wifiOpen: boolean;
  onWifiOpenChange: (open: boolean) => void;
  wifiInterfaces: NetworkInterface[];
  networkInterfaces: NetworkInterface[];
  confirmRecoverOpen: boolean;
  onConfirmRecoverOpenChange: (open: boolean) => void;
  recoverPending: boolean;
  onConfirmRecover: () => void;
}

// HotspotDialogs agrupa os dois dialogos modais da tela de hotspot
// (editar configuracao e confirmar recuperacao do adaptador Wi-Fi) -
// extraido de pages/Hotspot.tsx so para manter aquele arquivo dentro
// do limite de ~200 linhas deste repo (ver CLAUDE.md).
export function HotspotDialogs({
  configOpen,
  onConfigOpenChange,
  register,
  errors,
  handleSubmit,
  onSave,
  isDirty,
  savePending,
  showPassword,
  onToggleShowPassword,
  wifiOpen,
  onWifiOpenChange,
  wifiInterfaces,
  networkInterfaces,
  confirmRecoverOpen,
  onConfirmRecoverOpenChange,
  recoverPending,
  onConfirmRecover,
}: HotspotDialogsProps) {
  return (
    <>
      <Dialog open={configOpen} onOpenChange={onConfigOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Configuração do hotspot</DialogTitle>
            <DialogDescription>
              Salvar já recria o hotspot na hora para assumir os novos valores.
            </DialogDescription>
          </DialogHeader>
          <HotspotConfigForm
            register={register}
            errors={errors}
            handleSubmit={handleSubmit}
            onSave={onSave}
            isDirty={isDirty}
            savePending={savePending}
            showPassword={showPassword}
            onToggleShowPassword={onToggleShowPassword}
            wifiOpen={wifiOpen}
            onWifiOpenChange={onWifiOpenChange}
            wifiInterfaces={wifiInterfaces}
            networkInterfaces={networkInterfaces}
          />
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmRecoverOpen}
        onOpenChange={onConfirmRecoverOpenChange}
        title="Recuperar adaptador Wi-Fi"
        description="O hotspot está ligado e vai ser parado agora para recuperar o adaptador. Continuar?"
        confirmLabel="Recuperar"
        variant="destructive"
        pending={recoverPending}
        onConfirm={onConfirmRecover}
      />
    </>
  );
}
