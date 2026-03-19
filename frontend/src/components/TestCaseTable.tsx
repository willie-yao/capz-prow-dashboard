import { useState } from "react";
import type { TestCase } from "../types/dashboard";
import { formatDuration } from "../lib/utils";

interface TestCaseTableProps {
  testCases: TestCase[];
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

export function TestCaseTable({ testCases }: TestCaseTableProps) {
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());

  const sorted = [...testCases].sort(
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
                      {tc.name}
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
                    <div className="border-t border-outline-variant bg-error/5 px-6 py-3">
                      <pre className="whitespace-pre-wrap font-label text-xs leading-relaxed text-error">
                        {tc.failure_message}
                      </pre>
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
