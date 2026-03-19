import type { BuildResult } from "../types/dashboard";
import { dotColor } from "../lib/utils";

interface RunTimelineProps {
  runs: BuildResult[];
  selectedBuildId?: string;
  onSelect: (buildId: string) => void;
}

function shortDate(dateStr: string): string {
  const d = new Date(dateStr);
  return `${d.getMonth() + 1}/${d.getDate()}`;
}

export function RunTimeline({
  runs,
  selectedBuildId,
  onSelect,
}: RunTimelineProps) {
  // Oldest first so newest is on the right
  const sorted = [...runs].sort(
    (a, b) => new Date(a.started).getTime() - new Date(b.started).getTime()
  );

  return (
    <div className="overflow-x-auto">
      <div className="flex items-end gap-1 pb-5">
        {sorted.map((run, i) => {
          const isSelected = run.build_id === selectedBuildId;
          const showDate = i % 5 === 0 || i === sorted.length - 1;

          return (
            <div key={run.build_id} className="flex flex-col items-center">
              <button
                type="button"
                onClick={() => onSelect(run.build_id)}
                className={`h-4 w-4 rounded-sm transition-all ${dotColor(run.passed)} ${
                  isSelected
                    ? "ring-2 ring-primary ring-offset-1 ring-offset-surface"
                    : "hover:brightness-125"
                }`}
                title={`#${run.build_id} — ${run.passed ? "passed" : "failed"}`}
              />
              {showDate ? (
                <span className="mt-1.5 font-label text-[9px] text-on-surface-variant">
                  {shortDate(run.started)}
                </span>
              ) : (
                <span className="mt-1.5 h-3" />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
