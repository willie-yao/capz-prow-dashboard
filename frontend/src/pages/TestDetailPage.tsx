import React, { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useJobDetail } from "../hooks/useData";
import { formatDuration, timeAgo } from "../lib/utils";
import { DurationChart } from "../components/DurationChart";
import type { BuildResult, TestCase } from "../types/dashboard";

/** Strip numbers and hex strings to normalize error messages for grouping. */
function normalizeMessage(msg: string): string {
  return msg
    .replace(/0x[0-9a-fA-F]+/g, "…")
    .replace(/[0-9a-f]{8,}/gi, "…")
    .replace(/\d+/g, "…")
    .replace(/…[.…]+/g, "…")
    .trim();
}

/** Highlight Go file:line references in stack traces */
const goFileLineRe = /([a-zA-Z0-9_/.\-@]+\.go:\d+)/g;

function highlightStackTrace(body: string): (string | React.ReactElement)[] {
  const parts: (string | React.ReactElement)[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let key = 0;

  while ((match = goFileLineRe.exec(body)) !== null) {
    if (match.index > lastIndex) {
      parts.push(body.slice(lastIndex, match.index));
    }
    parts.push(
      <span key={key++} className="text-primary">
        {match[1]}
      </span>
    );
    lastIndex = match.index + match[0].length;
  }
  if (lastIndex < body.length) {
    parts.push(body.slice(lastIndex));
  }
  return parts;
}

interface TestOccurrence {
  run: BuildResult;
  testCase: TestCase | null; // null means absent from this run
}

interface FailureGroup {
  normalizedMessage: string;
  sampleMessage: string;
  count: number;
}

export function TestDetailPage() {
  const { jobName, testName: encodedTestName } = useParams<{
    jobName: string;
    testName: string;
  }>();
  const testName = encodedTestName ? decodeURIComponent(encodedTestName) : "";
  const { data, loading, error } = useJobDetail(jobName);
  const [selectedBuildId, setSelectedBuildId] = useState<string | null>(null);

  // Build per-run test occurrences (oldest first for timeline)
  const occurrences: TestOccurrence[] = useMemo(() => {
    if (!data) return [];
    const sorted = [...data.runs].sort(
      (a, b) => new Date(a.started).getTime() - new Date(b.started).getTime()
    );
    return sorted.map((run) => {
      const tc =
        run.test_cases.find((t) => t.name === testName) ?? null;
      return { run, testCase: tc };
    });
  }, [data, testName]);

  // Most recent occurrence that actually has this test
  const latestOccurrence = useMemo(() => {
    for (let i = occurrences.length - 1; i >= 0; i--) {
      if (occurrences[i].testCase) return occurrences[i];
    }
    return null;
  }, [occurrences]);

  // Failure classification
  const classification = useMemo(() => {
    if (!latestOccurrence) return null;
    // Count consecutive failures from the latest run backwards
    let consecutive = 0;
    for (let i = occurrences.length - 1; i >= 0; i--) {
      const tc = occurrences[i].testCase;
      if (!tc) continue; // skip runs where test wasn't present
      if (tc.status === "failed") consecutive++;
      else break;
    }
    if (consecutive === 0) return null;

    const failedRuns = occurrences.filter(
      (o) => o.testCase?.status === "failed"
    );
    const presentRuns = occurrences.filter((o) => o.testCase !== null);
    const passedRuns = presentRuns.filter(
      (o) => o.testCase!.status === "passed"
    );

    if (consecutive >= 3) return `Persistent (${consecutive}×)`;
    if (failedRuns.length > 1 && passedRuns.length > 0) return "Flaky";
    return "One-off";
  }, [occurrences, latestOccurrence]);

  // Failure pattern grouping
  const failureGroups: FailureGroup[] = useMemo(() => {
    const failures = occurrences.filter(
      (o) => o.testCase?.status === "failed" && o.testCase?.failure_message
    );
    if (failures.length === 0) return [];

    const groups = new Map<string, { sample: string; count: number }>();
    for (const f of failures) {
      const msg = f.testCase!.failure_message!;
      const key = normalizeMessage(msg);
      const existing = groups.get(key);
      if (existing) {
        existing.count++;
      } else {
        groups.set(key, { sample: msg, count: 1 });
      }
    }

    return Array.from(groups.entries())
      .map(([normalized, { sample, count }]) => ({
        normalizedMessage: normalized,
        sampleMessage: sample,
        count,
      }))
      .sort((a, b) => b.count - a.count);
  }, [occurrences]);

  const totalFailures = occurrences.filter(
    (o) => o.testCase?.status === "failed"
  ).length;

  // Selected run
  const effectiveSelectedId =
    selectedBuildId ?? latestOccurrence?.run.build_id ?? null;
  const selectedOccurrence = useMemo(() => {
    if (!effectiveSelectedId) return null;
    return (
      occurrences.find((o) => o.run.build_id === effectiveSelectedId) ?? null
    );
  }, [occurrences, effectiveSelectedId]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-32">
        <svg
          className="h-8 w-8 animate-spin text-primary"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle
            className="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeWidth="4"
          />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
          />
        </svg>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center gap-4 py-32 text-center">
        <p className="text-error text-lg">Failed to load job details</p>
        <p className="text-on-surface-variant text-sm">{error}</p>
        <button
          onClick={() => window.location.reload()}
          className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-on-primary transition-colors hover:bg-primary-dim"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!data) return null;

  const testFound = occurrences.some((o) => o.testCase !== null);
  if (!testFound) {
    return (
      <div className="space-y-8">
        <nav className="font-label flex items-center gap-2 text-sm text-on-surface-variant">
          <Link to="/" className="transition-colors hover:text-primary">
            Dashboard
          </Link>
          <span>›</span>
          <Link
            to={`/job/${encodeURIComponent(jobName ?? "")}`}
            className="transition-colors hover:text-primary"
          >
            {jobName}
          </Link>
          <span>›</span>
          <span className="text-on-surface truncate">{testName}</span>
        </nav>
        <div className="glass rounded-xl p-8 text-center">
          <p className="text-on-surface-variant">
            Test not found in any run of this job.
          </p>
        </div>
      </div>
    );
  }

  const latestStatus = latestOccurrence?.testCase?.status ?? "skipped";
  const selectedTc = selectedOccurrence?.testCase ?? null;
  const selectedRun = selectedOccurrence?.run ?? null;

  return (
    <div className="space-y-8">
      {/* Breadcrumb */}
      <nav className="font-label flex items-center gap-2 text-sm text-on-surface-variant">
        <Link to="/" className="transition-colors hover:text-primary">
          Dashboard
        </Link>
        <span>›</span>
        <Link
          to={`/job/${encodeURIComponent(jobName ?? "")}`}
          className="transition-colors hover:text-primary"
        >
          {jobName}
        </Link>
        <span>›</span>
        <span className="text-on-surface truncate max-w-md" title={testName}>
          {testName}
        </span>
      </nav>

      {/* Test header */}
      <div>
        <h1 className="font-headline text-2xl font-bold text-on-surface break-all">
          {testName}
        </h1>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <span
            className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${
              latestStatus === "passed"
                ? "bg-secondary/20 text-secondary"
                : latestStatus === "failed"
                  ? "bg-error/20 text-error"
                  : "bg-on-surface-variant/20 text-on-surface-variant"
            }`}
          >
            {latestStatus.charAt(0).toUpperCase() + latestStatus.slice(1)}
          </span>
          {classification && (
            <span
              className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${
                classification.startsWith("Persistent")
                  ? "bg-error/20 text-error"
                  : classification === "Flaky"
                    ? "bg-tertiary/20 text-tertiary"
                    : "bg-on-surface-variant/20 text-on-surface-variant"
              }`}
            >
              {classification}
            </span>
          )}
        </div>
      </div>

      {/* Pass/fail history bar */}
      <section>
        <h2 className="font-headline mb-3 text-lg font-semibold text-on-surface">
          History
        </h2>
        <div className="overflow-x-auto">
          <div className="flex items-start gap-1 p-1">
            {occurrences.map((occ, i) => {
              const tc = occ.testCase;
              const color = tc
                ? tc.status === "passed"
                  ? "bg-secondary"
                  : tc.status === "failed"
                    ? "bg-error"
                    : "bg-on-surface-variant"
                : "bg-on-surface-variant/30";
              const isSelected = occ.run.build_id === effectiveSelectedId;
              const showDate =
                i % 5 === 0 || i === occurrences.length - 1;
              const tooltip = tc
                ? `#${occ.run.build_id} — ${tc.status}`
                : `#${occ.run.build_id} — absent`;

              return (
                <div
                  key={occ.run.build_id}
                  className="flex flex-col items-center"
                >
                  <button
                    type="button"
                    onClick={() => setSelectedBuildId(occ.run.build_id)}
                    className={`h-4 w-4 rounded-sm transition-all ${color} ${
                      isSelected
                        ? "ring-2 ring-primary ring-offset-1 ring-offset-surface"
                        : "hover:brightness-125"
                    }`}
                    title={tooltip}
                  />
                  <span
                    className={`mt-1.5 font-label text-[9px] ${showDate ? "text-on-surface-variant" : "invisible"}`}
                  >
                    {shortDate(occ.run.started)}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      {/* Duration trend chart */}
      {(() => {
        const durationHistory = occurrences
          .filter((o) => o.testCase)
          .map((o) => ({
            build_id: o.run.build_id,
            timestamp: o.run.started,
            duration: o.testCase!.duration_seconds,
            passed: o.testCase!.status === "passed",
          }));
        return durationHistory.length > 0 ? (
          <section>
            <h2 className="font-headline mb-3 text-lg font-semibold text-on-surface">
              Duration Trend
            </h2>
            <div className="glass rounded-xl p-4">
              <DurationChart history={durationHistory} />
            </div>
          </section>
        ) : null;
      })()}

      {/* Failure pattern grouping */}
      {failureGroups.length > 0 && (
        <section>
          <h2 className="font-headline mb-3 text-lg font-semibold text-on-surface">
            Failure Patterns
          </h2>
          <div className="glass rounded-xl p-4 space-y-2">
            {failureGroups.map((group, i) => (
              <div
                key={i}
                className="flex items-start gap-3 text-sm"
              >
                <span className="shrink-0 rounded-full bg-error/20 px-2 py-0.5 font-label text-xs font-medium text-error">
                  {group.count} of {totalFailures}
                </span>
                <p className="min-w-0 truncate text-on-surface-variant" title={group.sampleMessage}>
                  {group.sampleMessage.length > 120
                    ? group.sampleMessage.slice(0, 120) + "…"
                    : group.sampleMessage}
                </p>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Selected failure detail */}
      {selectedRun && selectedTc && (
        <section className="glass rounded-xl p-6 space-y-5">
          <div className="flex items-center gap-3">
            <h3 className="font-headline text-base font-semibold text-on-surface">
              Run Detail
            </h3>
            <span
              className={`inline-block h-2.5 w-2.5 rounded-full ${
                selectedTc.status === "passed"
                  ? "bg-secondary"
                  : selectedTc.status === "failed"
                    ? "bg-error"
                    : "bg-on-surface-variant"
              }`}
            />
          </div>

          <div className="grid grid-cols-1 gap-x-8 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <div>
              <span className="font-label text-xs text-on-surface-variant">
                Build ID
              </span>
              <p className="text-on-surface">{selectedRun.build_id}</p>
            </div>
            <div>
              <span className="font-label text-xs text-on-surface-variant">
                Started
              </span>
              <p className="text-on-surface">
                {new Date(selectedRun.started).toLocaleString()}
              </p>
            </div>
            <div>
              <span className="font-label text-xs text-on-surface-variant">
                Duration
              </span>
              <p className="text-on-surface">
                {formatDuration(selectedTc.duration_seconds)}
              </p>
            </div>
            <div>
              <span className="font-label text-xs text-on-surface-variant">
                Run finished
              </span>
              <p className="text-on-surface">
                {timeAgo(selectedRun.finished)}
              </p>
            </div>
            <div className="flex items-end gap-3">
              {selectedRun.prow_url && (
                <a
                  href={selectedRun.prow_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary transition-colors hover:text-primary-dim"
                >
                  View in Prow ↗
                </a>
              )}
              {selectedRun.build_log_url && (
                <a
                  href={selectedRun.build_log_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary transition-colors hover:text-primary-dim"
                >
                  Build Log ↗
                </a>
              )}
            </div>
          </div>

          {/* Failure message */}
          {selectedTc.failure_message && (
            <pre className="whitespace-pre-wrap rounded-lg bg-error/5 p-4 font-label text-xs leading-relaxed text-error">
              {selectedTc.failure_message}
            </pre>
          )}

          {/* Full stack trace */}
          {selectedTc.failure_body && (
            <details className="group/trace [&>summary]:list-none [&>summary::-webkit-details-marker]:hidden">
              <summary className="cursor-pointer font-label text-xs text-on-surface-variant transition-colors hover:text-on-surface">
                <span className="inline-block transition-transform group-open/trace:rotate-90">▶</span> Stack Trace
              </summary>
              <pre className="mt-2 whitespace-pre-wrap font-label text-xs leading-relaxed text-on-surface-variant">
                {highlightStackTrace(selectedTc.failure_body)}
              </pre>
            </details>
          )}

          {/* Source location */}
          {selectedTc.failure_location && (
            <div className="flex items-center gap-2 text-xs">
              <span className="text-on-surface-variant">📍</span>
              {selectedTc.failure_location_url ? (
                <a
                  href={selectedTc.failure_location_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-mono text-primary hover:underline"
                >
                  {selectedTc.failure_location}
                </a>
              ) : (
                <span className="font-mono text-on-surface-variant">
                  {selectedTc.failure_location}
                </span>
              )}
            </div>
          )}

          {/* Cluster artifacts */}
          {selectedTc.cluster_artifacts && (
            <div className="rounded-lg border border-outline-variant bg-surface-container p-3 space-y-2">
              <p className="font-label text-xs font-medium text-on-surface">
                🔍 Debug Artifacts —{" "}
                {selectedTc.cluster_artifacts.cluster_name}
              </p>

              <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
                {selectedTc.cluster_artifacts.azure_activity_log && (
                  <a
                    href={selectedTc.cluster_artifacts.azure_activity_log}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:underline"
                  >
                    ☁️ Azure Activity Log
                  </a>
                )}
                {selectedTc.cluster_artifacts.bootstrap_resources_url && (
                  <a
                    href={selectedTc.cluster_artifacts.bootstrap_resources_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:underline"
                  >
                    📋 Cluster Resources
                  </a>
                )}
                {selectedTc.cluster_artifacts.pod_log_dirs && Object.entries(selectedTc.cluster_artifacts.pod_log_dirs).map(([dir, url]) => (
                  <a
                    key={dir}
                    href={url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:underline"
                  >
                    📦 {dir}
                  </a>
                ))}
              </div>

              {selectedTc.cluster_artifacts.machines &&
                selectedTc.cluster_artifacts.machines.length > 0 && (
                  <details className="group/machines [&>summary]:list-none [&>summary::-webkit-details-marker]:hidden">
                    <summary className="cursor-pointer font-label text-xs text-on-surface-variant transition-colors hover:text-on-surface">
                      <span className="inline-block transition-transform group-open/machines:rotate-90">▶</span> 🖥️ Machine Logs (
                      {selectedTc.cluster_artifacts.machines.length} machines)
                    </summary>
                    <div className="mt-2 space-y-2">
                      {selectedTc.cluster_artifacts.machines.map((m) => (
                        <div key={m.name} className="pl-4">
                          <p className="font-mono text-xs text-on-surface-variant">
                            {m.name}
                          </p>
                          <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1">
                            {Object.entries(m.logs).map(([logType, url]) => (
                              <a
                                key={logType}
                                href={url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="font-label text-[11px] text-primary hover:underline"
                              >
                                {logType}
                              </a>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  </details>
                )}
            </div>
          )}
        </section>
      )}

      {/* When a run is selected but the test wasn't present */}
      {selectedRun && !selectedTc && (
        <section className="glass rounded-xl p-8 text-center">
          <p className="text-on-surface-variant">
            This test was not present in build #{selectedRun.build_id}.
          </p>
        </section>
      )}
    </div>
  );
}

function shortDate(dateStr: string): string {
  const d = new Date(dateStr);
  return `${d.getMonth() + 1}/${d.getDate()}`;
}
