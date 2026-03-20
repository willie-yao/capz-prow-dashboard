import React, { useState } from "react";
import { Link } from "react-router-dom";
import type { TestCase } from "../types/dashboard";
import { formatDuration } from "../lib/utils";

interface TestCaseTableProps {
  testCases: TestCase[];
  jobName?: string;
}

const statusOrder: Record<string, number> = {
  failed: 0,
  passed: 1,
  skipped: 2,
};

function statusIcon(status: string) {
  switch (status) {
    case "passed":
      return <span className="text-secondary">✓</span>;
    case "failed":
      return <span className="text-error">✗</span>;
    default:
      return <span className="text-on-surface-variant">⊘</span>;
  }
}

// Hide Ginkgo setup/teardown entries unless they failed.
const setupPatterns = /synchronizedbeforesuite|synchronizedaftersuite|beforesuite|aftersuite/i;

// Highlight Go file:line references in stack traces
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

export function TestCaseTable({ testCases, jobName }: TestCaseTableProps) {
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());

  const filtered = testCases.filter(
    (tc) => tc.status !== "skipped" && (tc.status === "failed" || !setupPatterns.test(tc.name))
  );

  const sorted = [...filtered].sort(
    (a, b) => (statusOrder[a.status] ?? 3) - (statusOrder[b.status] ?? 3)
  );

  function toggleRow(idx: number) {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx);
      else next.add(idx);
      return next;
    });
  }

  return (
    <div className="overflow-x-auto rounded-xl border border-outline-variant">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-outline-variant bg-surface-container">
            <th className="w-10 px-3 py-2.5 text-left font-label text-xs uppercase tracking-wider text-on-surface-variant">
              &nbsp;
            </th>
            <th className="px-3 py-2.5 text-left font-label text-xs uppercase tracking-wider text-on-surface-variant">
              Test Name
            </th>
            <th className="w-24 px-3 py-2.5 text-right font-label text-xs uppercase tracking-wider text-on-surface-variant">
              Duration
            </th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((tc, idx) => {
            const isExpanded = expandedRows.has(idx);
            const hasFail = tc.status === "failed" && tc.failure_message;
            const stripe =
              idx % 2 === 0
                ? "bg-surface-container"
                : "bg-surface-container-high";

            return (
              <tr key={idx} className="group">
                <td colSpan={3} className="p-0">
                  <div
                    role={hasFail ? "button" : undefined}
                    tabIndex={hasFail ? 0 : undefined}
                    onClick={() => hasFail && toggleRow(idx)}
                    onKeyDown={(e) => {
                      if (hasFail && (e.key === "Enter" || e.key === " ")) {
                        e.preventDefault();
                        toggleRow(idx);
                      }
                    }}
                    className={`flex items-center ${stripe} ${hasFail ? "cursor-pointer hover:brightness-110" : ""}`}
                  >
                    <span className="w-10 shrink-0 px-3 py-2">
                      {statusIcon(tc.status)}
                    </span>
                    <span className="min-w-0 flex-1 truncate px-3 py-2 text-on-surface">
                      {jobName && tc.status === "failed" ? (
                        <Link
                          to={`/job/${encodeURIComponent(jobName)}/test/${encodeURIComponent(tc.name)}`}
                          className="hover:text-primary transition-colors"
                          onClick={(e) => e.stopPropagation()}
                        >
                          {tc.name}
                        </Link>
                      ) : (
                        tc.name
                      )}
                      {tc.failure_location_url && (
                        <a
                          href={tc.failure_location_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="ml-2 inline-flex text-primary hover:text-primary-container"
                          onClick={(e) => e.stopPropagation()}
                          title="View source on GitHub"
                        >
                          <svg
                            className="h-3.5 w-3.5"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            strokeWidth="2"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                          >
                            <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                            <polyline points="15 3 21 3 21 9" />
                            <line x1="10" y1="14" x2="21" y2="3" />
                          </svg>
                        </a>
                      )}
                    </span>
                    <span className="w-24 shrink-0 px-3 py-2 text-right font-label text-xs text-on-surface-variant">
                      {formatDuration(tc.duration_seconds)}
                    </span>
                  </div>

                  {hasFail && isExpanded && (
                    <div className="border-t border-outline-variant bg-error/5 px-6 py-4 space-y-3">
                      {/* Failure message */}
                      <pre className="whitespace-pre-wrap font-label text-xs leading-relaxed text-error">
                        {tc.failure_message}
                      </pre>

                      {/* Full stack trace */}
                      {tc.failure_body && (
                        <details className="group/trace">
                          <summary className="cursor-pointer font-label text-xs text-on-surface-variant hover:text-on-surface transition-colors">
                            ▶ Stack Trace
                          </summary>
                          <pre className="mt-2 whitespace-pre-wrap font-label text-xs leading-relaxed text-on-surface-variant">
                            {highlightStackTrace(tc.failure_body)}
                          </pre>
                        </details>
                      )}

                      {/* Source location link */}
                      {tc.failure_location && (
                        <div className="flex items-center gap-2 text-xs">
                          <span className="text-on-surface-variant">📍</span>
                          {tc.failure_location_url ? (
                            <a
                              href={tc.failure_location_url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="font-mono text-primary hover:underline"
                            >
                              {tc.failure_location}
                            </a>
                          ) : (
                            <span className="font-mono text-on-surface-variant">
                              {tc.failure_location}
                            </span>
                          )}
                        </div>
                      )}

                      {/* Cluster artifact links */}
                      {tc.cluster_artifacts && (
                        <div className="rounded-lg border border-outline-variant bg-surface-container p-3 space-y-2">
                          <p className="font-label text-xs font-medium text-on-surface">
                            🔍 Debug Artifacts — {tc.cluster_artifacts.cluster_name}
                          </p>

                          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
                            {tc.cluster_artifacts.azure_activity_log && (
                              <a
                                href={tc.cluster_artifacts.azure_activity_log}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-primary hover:underline"
                              >
                                ☁️ Azure Activity Log
                              </a>
                            )}
                            {tc.cluster_artifacts.pod_log_dirs?.map((dir) => (
                              <span key={dir} className="text-on-surface-variant">
                                📦 {dir}
                              </span>
                            ))}
                          </div>

                          {tc.cluster_artifacts.machines && tc.cluster_artifacts.machines.length > 0 && (
                            <details className="group/machines">
                              <summary className="cursor-pointer font-label text-xs text-on-surface-variant hover:text-on-surface transition-colors">
                                🖥️ Machine Logs ({tc.cluster_artifacts.machines.length} machines)
                              </summary>
                              <div className="mt-2 space-y-2">
                                {tc.cluster_artifacts.machines.map((m) => (
                                  <div key={m.name} className="pl-4">
                                    <p className="font-mono text-xs text-on-surface-variant">{m.name}</p>
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
                    </div>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
