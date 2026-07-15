import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { bytesToGB, formatQuotaValue, LIMIT_TYPE_LABELS } from "@/components/hotspot/hotspot-limits-types";
import { HotspotProfileForm } from "@/components/hotspot/HotspotProfileForm";
import { useHotspotProfiles } from "@/components/hotspot/useHotspotProfileQueries";
import { useHotspotProfileMutations } from "@/components/hotspot/useHotspotProfileMutations";
import type { HotspotProfile, HotspotProfileRequest } from "@/components/hotspot/hotspot-profile-types";

const emptyProfile: HotspotProfile = {
  id: "",
  name: "",
  isDefault: false,
  downloadRateValue: null,
  downloadRateUnit: "mbit",
  uploadRateValue: null,
  uploadRateUnit: "mbit",
  limitType: "unlimited",
  dailyQuotaBytes: null,
  dailyQuotaUnit: "gbyte",
  weeklyQuotaBytes: null,
  weeklyQuotaUnit: "gbyte",
  monthlyQuotaBytes: null,
  monthlyQuotaUnit: "gbyte",
  creditRechargeAmountBytes: null,
  creditRechargePeriod: null,
  creditPlafondBytes: null,
};

function rateSummary(profile: HotspotProfile) {
  if (!profile.downloadRateValue && !profile.uploadRateValue) return "sem limite";
  return `${profile.downloadRateValue ?? "-"} / ${profile.uploadRateValue ?? "-"} ${profile.downloadRateUnit}`;
}

function typeDetailSummary(profile: HotspotProfile) {
  if (profile.limitType === "credit") {
    return profile.creditRechargeAmountBytes
      ? `recarga ${bytesToGB(profile.creditRechargeAmountBytes).toFixed(0)}GB/${profile.creditRechargePeriod}`
      : "só recarga manual";
  }
  if (profile.limitType === "quota") {
    const periods: [number | null, string, typeof profile.dailyQuotaUnit][] = [
      [profile.dailyQuotaBytes, "diária", profile.dailyQuotaUnit],
      [profile.weeklyQuotaBytes, "semanal", profile.weeklyQuotaUnit],
      [profile.monthlyQuotaBytes, "mensal", profile.monthlyQuotaUnit],
    ];
    const configured = periods.filter(([bytes]) => bytes !== null) as [number, string, typeof profile.dailyQuotaUnit][];
    if (configured.length === 0) return "sem teto definido";
    return configured.map(([bytes, label, unit]) => `${formatQuotaValue(bytes, unit)}/${label}`).join(", ");
  }
  if (profile.limitType === "custom") return "dispositivo define";
  return "—";
}

export function HotspotProfilesCard() {
  const profiles = useHotspotProfiles();
  const { create, update, remove } = useHotspotProfileMutations();
  const [editing, setEditing] = useState<HotspotProfile | null>(null);
  const [deleting, setDeleting] = useState<HotspotProfile | null>(null);

  function onSubmit(values: HotspotProfileRequest) {
    if (editing?.id) {
      update.mutate({ id: editing.id, profile: values }, { onSuccess: () => setEditing(null) });
    } else {
      create.mutate(values, { onSuccess: () => setEditing(null) });
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <CardTitle>Perfis de dispositivo</CardTitle>
        <Button size="sm" onClick={() => setEditing(emptyProfile)}>
          <Plus className="h-4 w-4" />
          Novo perfil
        </Button>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nome</TableHead>
              <TableHead className="hidden sm:table-cell">Taxa</TableHead>
              <TableHead className="hidden sm:table-cell">Tipo</TableHead>
              <TableHead className="hidden md:table-cell">Detalhe</TableHead>
              <TableHead className="text-right">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(profiles.data ?? []).map((profile) => (
              <TableRow key={profile.id}>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <span className="font-medium">{profile.name}</span>
                    {profile.isDefault && <Badge variant="secondary">Padrão</Badge>}
                  </div>
                </TableCell>
                <TableCell className="hidden text-sm sm:table-cell">{rateSummary(profile)}</TableCell>
                <TableCell className="hidden text-sm sm:table-cell">{LIMIT_TYPE_LABELS[profile.limitType]}</TableCell>
                <TableCell className="hidden text-sm md:table-cell">{typeDetailSummary(profile)}</TableCell>
                <TableCell>
                  <div className="flex justify-end gap-2">
                    <Button variant="outline" size="sm" onClick={() => setEditing(profile)}>
                      Editar
                    </Button>
                    {!profile.isDefault && (
                      <Button variant="outline" size="sm" onClick={() => setDeleting(profile)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {profiles.data?.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-muted-foreground">
                  Nenhum perfil cadastrado.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>

      <Dialog open={editing !== null} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{editing?.id ? "Editar perfil" : "Novo perfil"}</DialogTitle>
          </DialogHeader>
          {editing && (
            <HotspotProfileForm
              value={editing}
              onSubmit={onSubmit}
              pending={create.isPending || update.isPending}
            />
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title="Remover perfil"
        description={`Dispositivos vinculados a "${deleting?.name}" passam a usar o perfil Padrão. Continuar?`}
        confirmLabel="Remover"
        variant="destructive"
        pending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id, { onSuccess: () => setDeleting(null) })}
      />
    </Card>
  );
}
