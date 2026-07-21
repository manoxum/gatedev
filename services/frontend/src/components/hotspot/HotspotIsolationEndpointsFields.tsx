import type { FieldErrors, UseFormRegister, UseFormSetValue } from "react-hook-form";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import type { HotspotCommRuleFormValues } from "@/components/hotspot/hotspot-isolation-schema";
import type { CommEndpointOption } from "@/components/hotspot/HotspotIsolationRuleDialog";

interface HotspotIsolationEndpointsFieldsProps {
  register: UseFormRegister<HotspotCommRuleFormValues>;
  setValue: UseFormSetValue<HotspotCommRuleFormValues>;
  errors: FieldErrors<HotspotCommRuleFormValues>;
  sourceKind: string;
  targetKind: string;
  profileOptions: CommEndpointOption[];
  deviceOptions: CommEndpointOption[];
}

// Campos da modalidade "entre origem e destino" - extraidos do
// HotspotIsolationRuleDialog para manter aquele arquivo dentro do limite
// de ~200 linhas. Usa os metodos do mesmo react-hook-form do pai (eles
// atravessam a borda de componente normalmente).
export function HotspotIsolationEndpointsFields({
  register,
  setValue,
  errors,
  sourceKind,
  targetKind,
  profileOptions,
  deviceOptions,
}: HotspotIsolationEndpointsFieldsProps) {
  const optionsFor = (kind: string) => (kind === "device" ? deviceOptions : profileOptions);

  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label>Origem</Label>
          <SelectNative {...register("sourceKind", { onChange: () => setValue("sourceRef", "") })} aria-label="Tipo da origem">
            <option value="profile">Perfil</option>
            <option value="device">Cliente</option>
          </SelectNative>
          <SelectNative {...register("sourceRef")} aria-label="Origem">
            <option value="">Escolha…</option>
            {optionsFor(sourceKind).map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </SelectNative>
          {errors.sourceRef && <p className="text-sm text-destructive">{errors.sourceRef.message}</p>}
        </div>
        <div className="space-y-2">
          <Label>Destino</Label>
          <SelectNative {...register("targetKind", { onChange: () => setValue("targetRef", "") })} aria-label="Tipo do destino">
            <option value="profile">Perfil</option>
            <option value="device">Cliente</option>
            <option value="any">Todos os clientes</option>
          </SelectNative>
          {targetKind !== "any" && (
            <SelectNative {...register("targetRef")} aria-label="Destino">
              <option value="">Escolha…</option>
              {optionsFor(targetKind).map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </SelectNative>
          )}
          {errors.targetRef && <p className="text-sm text-destructive">{errors.targetRef.message}</p>}
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="directionUi">Sentido</Label>
        <SelectNative id="directionUi" {...register("directionUi")}>
          <option value="both">Ambos podem iniciar</option>
          <option value="to">Só a origem inicia</option>
          {targetKind !== "any" && <option value="from">Só o destino inicia</option>}
        </SelectNative>
        <p className="text-xs text-muted-foreground">
          Quem inicia abre a comunicação; as respostas voltam automaticamente.
        </p>
      </div>
    </>
  );
}
