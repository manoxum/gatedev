export type QuotaPeriod = "daily" | "weekly" | "monthly";

export interface HotspotLimits {
  downloadRateMbps: number | null;
  uploadRateMbps: number | null;
  quotaBytes: number | null;
  quotaPeriod: QuotaPeriod | null;
  quotaThrottleDownloadMbps: number | null;
  quotaThrottleUploadMbps: number | null;
}

export interface HotspotTraffic {
  downloadBytes: number;
  uploadBytes: number;
  periodStart: string;
  periodEnd: string;
  throttled: boolean;
  quotaBytes: number | null;
  quotaPeriod: QuotaPeriod | null;
}

export const GIGABYTE = 1024 * 1024 * 1024;

export function bytesToGB(bytes: number) {
  return bytes / GIGABYTE;
}
