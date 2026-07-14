import { z } from "zod";
import {
  quotaValueToBytes,
  bytesToQuotaValue,
  type HotspotGlobalLimits,
  type QuotaPeriod,
  type RateUnit,
} from "@/components/hotspot/hotspot-limits-types";

const optionalPositiveInt = z
  .string()
  .trim()
  .refine((value) => value === "" || (/^\d+$/.test(value) && Number(value) > 0), "Deve ser um número positivo");

const rateUnit = z.enum(["kbit", "mbit", "gbit", "kbyte", "mbyte", "gbyte"]);

// So o limite global usa este schema/shape (cota unica + throttle) -
// device/perfil usam hotspot-device-limits-schema.ts (tipo unico
// ilimitado/credito/cota).
export const hotspotGlobalLimitsFormSchema = z.object({
  downloadRateValue: optionalPositiveInt,
  downloadRateUnit: rateUnit,
  uploadRateValue: optionalPositiveInt,
  uploadRateUnit: rateUnit,
  quotaValue: optionalPositiveInt,
  quotaUnit: rateUnit,
  quotaPeriod: z.enum(["daily", "weekly", "monthly"]),
  quotaThrottleDownloadValue: optionalPositiveInt,
  quotaThrottleDownloadUnit: rateUnit,
  quotaThrottleUploadValue: optionalPositiveInt,
  quotaThrottleUploadUnit: rateUnit,
});

export type HotspotGlobalLimitsFormValues = z.infer<typeof hotspotGlobalLimitsFormSchema>;

export function globalLimitsToFormValues(limits: HotspotGlobalLimits): HotspotGlobalLimitsFormValues {
  return {
    downloadRateValue: limits.downloadRateValue?.toString() ?? "",
    downloadRateUnit: limits.downloadRateUnit,
    uploadRateValue: limits.uploadRateValue?.toString() ?? "",
    uploadRateUnit: limits.uploadRateUnit,
    quotaValue: limits.quotaBytes ? String(Math.round(bytesToQuotaValue(limits.quotaBytes, limits.quotaUnit))) : "",
    quotaUnit: limits.quotaUnit,
    quotaPeriod: limits.quotaPeriod ?? "daily",
    quotaThrottleDownloadValue: limits.quotaThrottleDownloadValue?.toString() ?? "",
    quotaThrottleDownloadUnit: limits.quotaThrottleDownloadUnit,
    quotaThrottleUploadValue: limits.quotaThrottleUploadValue?.toString() ?? "",
    quotaThrottleUploadUnit: limits.quotaThrottleUploadUnit,
  };
}

export function formValuesToGlobalLimits(values: HotspotGlobalLimitsFormValues): HotspotGlobalLimits {
  const hasQuota = values.quotaValue !== "";
  return {
    downloadRateValue: values.downloadRateValue ? Number(values.downloadRateValue) : null,
    downloadRateUnit: values.downloadRateUnit as RateUnit,
    uploadRateValue: values.uploadRateValue ? Number(values.uploadRateValue) : null,
    uploadRateUnit: values.uploadRateUnit as RateUnit,
    quotaBytes: hasQuota ? quotaValueToBytes(Number(values.quotaValue), values.quotaUnit as RateUnit) : null,
    quotaUnit: values.quotaUnit as RateUnit,
    quotaPeriod: hasQuota ? (values.quotaPeriod as QuotaPeriod) : null,
    quotaThrottleDownloadValue: values.quotaThrottleDownloadValue ? Number(values.quotaThrottleDownloadValue) : null,
    quotaThrottleDownloadUnit: values.quotaThrottleDownloadUnit as RateUnit,
    quotaThrottleUploadValue: values.quotaThrottleUploadValue ? Number(values.quotaThrottleUploadValue) : null,
    quotaThrottleUploadUnit: values.quotaThrottleUploadUnit as RateUnit,
  };
}
