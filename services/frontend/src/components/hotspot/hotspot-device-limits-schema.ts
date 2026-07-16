import { z } from "zod";
import {
  quotaValueToBytes,
  bytesToQuotaValue,
  type HotspotLimits,
  type LimitType,
  type RateUnit,
} from "@/components/hotspot/hotspot-limits-types";

const optionalPositiveInt = z
  .string()
  .trim()
  .refine((value) => value === "" || (/^\d+$/.test(value) && Number(value) > 0), "Deve ser um número positivo");

const rateUnit = z.enum(["kbit", "mbit", "gbit", "kbyte", "mbyte", "gbyte"]);

// Schema do shape novo (tipo unico ilimitado/credito/cota[/customizado]) -
// usado por dispositivo (override) e perfil, ver HotspotLimitTypeFields.tsx.
// "custom" so e oferecido como opcao no formulario de perfil
// (HotspotLimitTypeToggle com includeCustom) - o schema aceita o valor
// nos dois casos por simplicidade (um so enum), ja que o formulario de
// dispositivo nunca produz esse valor.
export const hotspotLimitsFormSchema = z.object({
  downloadRateValue: optionalPositiveInt,
  downloadRateUnit: rateUnit,
  uploadRateValue: optionalPositiveInt,
  uploadRateUnit: rateUnit,
  limitType: z.enum(["unlimited", "credit", "quota", "custom"]),
  dailyQuotaValue: optionalPositiveInt,
  dailyQuotaUnit: rateUnit,
  weeklyQuotaValue: optionalPositiveInt,
  weeklyQuotaUnit: rateUnit,
  monthlyQuotaValue: optionalPositiveInt,
  monthlyQuotaUnit: rateUnit,
});

export type HotspotLimitsFormValues = z.infer<typeof hotspotLimitsFormSchema>;

export function limitsToFormValues(limits: HotspotLimits): HotspotLimitsFormValues {
  return {
    downloadRateValue: limits.downloadRateValue?.toString() ?? "",
    downloadRateUnit: limits.downloadRateUnit,
    uploadRateValue: limits.uploadRateValue?.toString() ?? "",
    uploadRateUnit: limits.uploadRateUnit,
    limitType: limits.limitType,
    dailyQuotaValue: limits.dailyQuotaBytes
      ? String(Math.round(bytesToQuotaValue(limits.dailyQuotaBytes, limits.dailyQuotaUnit)))
      : "",
    dailyQuotaUnit: limits.dailyQuotaUnit,
    weeklyQuotaValue: limits.weeklyQuotaBytes
      ? String(Math.round(bytesToQuotaValue(limits.weeklyQuotaBytes, limits.weeklyQuotaUnit)))
      : "",
    weeklyQuotaUnit: limits.weeklyQuotaUnit,
    monthlyQuotaValue: limits.monthlyQuotaBytes
      ? String(Math.round(bytesToQuotaValue(limits.monthlyQuotaBytes, limits.monthlyQuotaUnit)))
      : "",
    monthlyQuotaUnit: limits.monthlyQuotaUnit,
  };
}

export function formValuesToLimits(values: HotspotLimitsFormValues): HotspotLimits {
  return {
    downloadRateValue: values.downloadRateValue ? Number(values.downloadRateValue) : null,
    downloadRateUnit: values.downloadRateUnit as RateUnit,
    uploadRateValue: values.uploadRateValue ? Number(values.uploadRateValue) : null,
    uploadRateUnit: values.uploadRateUnit as RateUnit,
    limitType: values.limitType as LimitType,
    dailyQuotaBytes: values.dailyQuotaValue
      ? quotaValueToBytes(Number(values.dailyQuotaValue), values.dailyQuotaUnit as RateUnit)
      : null,
    dailyQuotaUnit: values.dailyQuotaUnit as RateUnit,
    weeklyQuotaBytes: values.weeklyQuotaValue
      ? quotaValueToBytes(Number(values.weeklyQuotaValue), values.weeklyQuotaUnit as RateUnit)
      : null,
    weeklyQuotaUnit: values.weeklyQuotaUnit as RateUnit,
    monthlyQuotaBytes: values.monthlyQuotaValue
      ? quotaValueToBytes(Number(values.monthlyQuotaValue), values.monthlyQuotaUnit as RateUnit)
      : null,
    monthlyQuotaUnit: values.monthlyQuotaUnit as RateUnit,
  };
}
