import type { JobSummary } from "../types/dashboard";

export function statusColor(status: string): string {
  switch (status) {
    case "PASSING":
      return "text-secondary";
    case "FAILING":
      return "text-error";
    case "FLAKY":
      return "text-tertiary";
    default:
      return "text-on-surface-variant";
  }
}

export function statusBg(status: string): string {
  switch (status) {
    case "PASSING":
      return "bg-secondary";
    case "FAILING":
      return "bg-error";
    case "FLAKY":
      return "bg-tertiary";
    default:
      return "bg-on-surface-variant";
  }
}

export function dotColor(passed: boolean, result?: string): string {
  if (result === "PENDING") return "bg-tertiary";
  return passed ? "bg-secondary" : "bg-error";
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  const h = Math.floor(seconds / 3600);
  const m = Math.round((seconds % 3600) / 60);
  return `${h}h ${m}m`;
}

export function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const hours = Math.floor(diff / 3600000);
  if (hours < 1) return "just now";
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days === 1) return "1 day ago";
  return `${days} days ago`;
}

export function formatPercent(rate: number): string {
  return `${Math.round(rate * 100)}%`;
}

export function groupByCategory(
  jobs: JobSummary[]
): Record<string, JobSummary[]> {
  const groups: Record<string, JobSummary[]> = {};
  for (const job of jobs) {
    const cat = job.category || "other";
    (groups[cat] ??= []).push(job);
  }
  return groups;
}

export function groupByBranch(
  jobs: JobSummary[]
): Record<string, JobSummary[]> {
  const groups: Record<string, JobSummary[]> = {};
  for (const job of jobs) {
    const branch = job.branch || "unknown";
    (groups[branch] ??= []).push(job);
  }
  return groups;
}

export const categoryLabels: Record<string, string> = {
  "capz-e2e": "CAPZ E2E",
  "aks-e2e": "AKS E2E",
  upgrade: "Upgrade",
  "capi-e2e": "CAPI E2E",
  conformance: "Conformance",
  coverage: "Coverage",
  scalability: "Scalability",
  other: "Other",
};
