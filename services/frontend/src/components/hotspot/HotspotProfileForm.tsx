import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { HotspotLimitTypeFields } from "@/components/hotspot/HotspotLimitTypeFields";
import {
  hotspotProfileFormSchema,
  profileToFormValues,
  formValuesToProfile,
  type HotspotProfileFormValues,
} from "@/components/hotspot/hotspot-profile-schema";
import type { HotspotProfile, HotspotProfileRequest } from "@/components/hotspot/hotspot-profile-types";

interface HotspotProfileFormProps {
  value: HotspotProfile;
  onSubmit: (profile: HotspotProfileRequest) => void;
  pending: boolean;
}

// Formulario de perfil - mesmo shape de campos de taxa/tipo/cota do
// limite de dispositivo (via HotspotLimitTypeFields, com a politica de
// recarga de credito inline - perfil funde tudo num so formulario) +
// nome.
export function HotspotProfileForm({ value, onSubmit, pending }: HotspotProfileFormProps) {
  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors, isDirty },
  } = useForm<HotspotProfileFormValues>({
    resolver: zodResolver(hotspotProfileFormSchema),
    values: profileToFormValues(value),
  });
  const limitType = watch("limitType");

  return (
    <form className="space-y-6" onSubmit={handleSubmit((values) => onSubmit(formValuesToProfile(values)))}>
      <div className="space-y-2">
        <Label htmlFor="name">Nome</Label>
        <Input id="name" placeholder="ex.: Convidados" {...register("name")} disabled={value.isDefault} />
        {errors.name && <p className="text-sm text-destructive">{errors.name.message}</p>}
      </div>

      <HotspotLimitTypeFields
        register={register}
        errors={errors}
        limitType={limitType}
        onLimitTypeChange={(type) => setValue("limitType", type, { shouldDirty: true, shouldValidate: true })}
        includeCustom
      />

      <Button type="submit" disabled={!isDirty || pending}>
        Salvar
      </Button>
    </form>
  );
}
