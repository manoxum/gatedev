import { z } from "zod";
import {
  quotaValueToBytes,
  bytesToQuotaValue,
  type HotspotLimits,
  type QuotaPeriod,
  type RateUnit,
} from "@/components/hotspot/hotspot-limits-types";

const optionalPositiveInt = z
  .string()
  .trim()
  .refine((value) => value === "" || (/^\d+$/.test(value) && Number(value) > 0), "Deve ser um número positivo");

const rateUnit = z.enum(["kbit", "mbit", "gbit", "kbyte", "mbyte", "gbyte"]);

export const hotspotLimitsFormSchema = z.object({
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

export type HotspotLimitsFormValues = z.infer<typeof hotspotLimitsFormSchema>;

export function limitsToFormValues(limits: HotspotLimits): HotspotLimitsFormValues {
  return {
    downloadRateValue: limits.downloadRateValue?.toString() ?? "",
    downloadRateUnit: limits.downloadRateUnit,
    uploadRateValue: limits.uploadRateValue?.toString() ?? "",
    uploadRateUnit: limits.uploadRateUnit,
    quotaValue: limits.quotaBytes ? String(Math.round(bytesToQuotaValue(limits.quotaBytes, "gbyte"))) : "",
    quotaUnit: "gbyte",
    quotaPeriod: limits.quotaPeriod ?? "daily",
    quotaThrottleDownloadValue: limits.quotaThrottleDownloadValue?.toString() ?? "",
    quotaThrottleDownloadUnit: limits.quotaThrottleDownloadUnit,
    quotaThrottleUploadValue: limits.quotaThrottleUploadValue?.toString() ?? "",
    quotaThrottleUploadUnit: limits.quotaThrottleUploadUnit,
  };
}

export function formValuesToLimits(values: HotspotLimitsFormValues): HotspotLimits {
  const hasQuota = values.quotaValue !== "";
  return {
    downloadRateValue: values.downloadRateValue ? Number(values.downloadRateValue) : null,
    downloadRateUnit: values.downloadRateUnit as RateUnit,
    uploadRateValue: values.uploadRateValue ? Number(values.uploadRateValue) : null,
    uploadRateUnit: values.uploadRateUnit as RateUnit,
    quotaBytes: hasQuota ? quotaValueToBytes(Number(values.quotaValue), values.quotaUnit as RateUnit) : null,
    quotaPeriod: hasQuota ? (values.quotaPeriod as QuotaPeriod) : null,
    quotaThrottleDownloadValue: values.quotaThrottleDownloadValue ? Number(values.quotaThrottleDownloadValue) : null,
    quotaThrottleDownloadUnit: values.quotaThrottleDownloadUnit as RateUnit,
    quotaThrottleUploadValue: values.quotaThrottleUploadValue ? Number(values.quotaThrottleUploadValue) : null,
    quotaThrottleUploadUnit: values.quotaThrottleUploadUnit as RateUnit,
  };
}
