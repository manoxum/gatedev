import { z } from "zod";
import { GIGABYTE, type HotspotLimits, type QuotaPeriod } from "@/components/hotspot/hotspot-limits-types";

const optionalPositiveInt = z
  .string()
  .trim()
  .refine((value) => value === "" || (/^\d+$/.test(value) && Number(value) > 0), "Deve ser um número positivo");

export const hotspotLimitsFormSchema = z.object({
  downloadRateMbps: optionalPositiveInt,
  uploadRateMbps: optionalPositiveInt,
  quotaGB: optionalPositiveInt,
  quotaPeriod: z.enum(["daily", "weekly", "monthly"]),
  quotaThrottleDownloadMbps: optionalPositiveInt,
  quotaThrottleUploadMbps: optionalPositiveInt,
});

export type HotspotLimitsFormValues = z.infer<typeof hotspotLimitsFormSchema>;

export function limitsToFormValues(limits: HotspotLimits): HotspotLimitsFormValues {
  return {
    downloadRateMbps: limits.downloadRateMbps?.toString() ?? "",
    uploadRateMbps: limits.uploadRateMbps?.toString() ?? "",
    quotaGB: limits.quotaBytes ? String(Math.round(limits.quotaBytes / GIGABYTE)) : "",
    quotaPeriod: limits.quotaPeriod ?? "daily",
    quotaThrottleDownloadMbps: limits.quotaThrottleDownloadMbps?.toString() ?? "",
    quotaThrottleUploadMbps: limits.quotaThrottleUploadMbps?.toString() ?? "",
  };
}

export function formValuesToLimits(values: HotspotLimitsFormValues): HotspotLimits {
  const hasQuota = values.quotaGB !== "";
  return {
    downloadRateMbps: values.downloadRateMbps ? Number(values.downloadRateMbps) : null,
    uploadRateMbps: values.uploadRateMbps ? Number(values.uploadRateMbps) : null,
    quotaBytes: hasQuota ? Number(values.quotaGB) * GIGABYTE : null,
    quotaPeriod: hasQuota ? (values.quotaPeriod as QuotaPeriod) : null,
    quotaThrottleDownloadMbps: values.quotaThrottleDownloadMbps ? Number(values.quotaThrottleDownloadMbps) : null,
    quotaThrottleUploadMbps: values.quotaThrottleUploadMbps ? Number(values.quotaThrottleUploadMbps) : null,
  };
}
