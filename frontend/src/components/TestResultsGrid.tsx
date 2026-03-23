import { useMemo } from "react";
import { Link } from "react-router-dom";
import type { BuildResult } from "../types/dashboard";

interface TestResultsGridProps {
  runs: BuildResult[];
  jobName: string;
}

type CellStatus = "passed" | "failed" | "skipped";

interface GridRow {
  testName: string;
  failCount: number;
  cells: CellStatus[];
}

const setupPatterns =
  /^(SynchronizedBeforeSuite|SynchronizedAfterSuite|BeforeSuite|AfterSuite)$/i;

function shortDate(dateStr: string): string {
  const d = new Date(dateStr);
  return `${d.getMonth() + 1}/${d.getDate()}`;
}

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) + "…" : s;
}

export function TestResultsGrid({ runs, jobName }: TestResultsGridProps) {
  // Sort runs oldest→newest (left to right)
  const sortedRuns = useMemo(
    () =>
      [...runs].sort(
        (a, b) =>
          new Date(a.started).getTime() - new Date(b.started).getTime(),
      ),
    [runs],
  );

  const gridRows = useMemo(() => {
    if (sortedRuns.length === 0) return [];

    // Build a map: testName → status per run index
    const testMap = new Map<string, CellStatus[]>();

    for (let col = 0; col < sortedRuns.length; col++) {
      const run = sortedRuns[col];
      for (const tc of run.test_cases) {
        if (!testMap.has(tc.name)) {
          testMap.set(tc.name, new Array(sortedRuns.length).fill("skipped"));
        }
        testMap.get(tc.name)![col] = tc.status;
      }
    }

    // Filter and build rows
    const rows: GridRow[] = [];

    for (const [testName, cells] of testMap) {
      const failCount = cells.filter((s) => s === "failed").length;
      const hasPass = cells.some((s) => s === "passed");
      const hasFail = failCount > 0;

      // Filter out tests that passed in ALL runs
      if (!hasFail) continue;

      // Filter out skipped-only tests
      if (!hasPass && !hasFail) continue;

      // Filter out setup/teardown unless they failed
      if (setupPatterns.test(testName) && !hasFail) continue;

      rows.push({ testName, failCount, cells });
    }

    // Sort: most failures first, then alphabetical
    rows.sort((a, b) => {
      if (b.failCount !== a.failCount) return b.failCount - a.failCount;
      return a.testName.localeCompare(b.testName);
    });

    return rows;
  }, [sortedRuns]);

  if (runs.length === 0 || gridRows.length === 0) {
    return (
      <div className="glass rounded-xl p-6 text-center">
        <p className="text-sm text-on-surface-variant">
          {runs.length === 0
            ? "No runs available."
            : "All tests passed across all runs — nothing to display."}
        </p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-xl border border-outline-variant bg-surface">
      <table className="border-collapse">
        {/* Column headers: date labels */}
        <thead>
          <tr>
            <th className="sticky left-0 z-10 bg-surface px-3 py-2" />
            {sortedRuns.map((run) => (
              <th
                key={run.build_id}
                className="px-0.5 py-2 font-label text-[10px] font-normal text-on-surface-variant"
              >
                {shortDate(run.started)}
              </th>
            ))}
          </tr>
        </thead>

        <tbody>
          {gridRows.map((row) => (
            <tr key={row.testName} className="group hover:brightness-110">
              {/* Sticky test name column */}
              <td className="sticky left-0 z-10 border-r border-outline-variant bg-surface px-3 py-1 group-hover:brightness-110">
                <Link
                  to={`/job/${encodeURIComponent(jobName)}/test/${encodeURIComponent(row.testName)}`}
                  className="block max-w-[260px] truncate text-xs text-on-surface transition-colors hover:text-primary"
                  title={row.testName}
                >
                  {truncate(row.testName, 40)}
                </Link>
              </td>

              {/* Status cells */}
              {row.cells.map((status, colIdx) => {
                const run = sortedRuns[colIdx];
                const cellColor =
                  status === "passed"
                    ? "bg-secondary"
                    : status === "failed"
                      ? "bg-error"
                      : "bg-on-surface-variant/30";

                const cell = (
                  <div
                    className={`mx-auto h-3 w-3 rounded-[2px] ${cellColor}`}
                    title={`${row.testName}\n#${run.build_id} — ${status}`}
                  />
                );

                return (
                  <td key={run.build_id} className="px-0.5 py-0.5">
                    {status === "failed" ? (
                      <Link
                        to={`/job/${encodeURIComponent(jobName)}?run=${run.build_id}`}
                      >
                        {cell}
                      </Link>
                    ) : (
                      cell
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
