import type { JobSummary } from "../types/dashboard";

interface SummaryBarProps {
  jobs: JobSummary[];
}

export function SummaryBar({ jobs }: SummaryBarProps) {
  const passing = jobs.filter((j) => j.overall_status === "PASSING").length;
  const flaky = jobs.filter((j) => j.overall_status === "FLAKY").length;
  const failing = jobs.filter((j) => j.overall_status === "FAILING").length;

  const cards = [
    { label: "Passing", count: passing, text: "text-secondary", bg: "bg-secondary/10" },
    { label: "Flaky", count: flaky, text: "text-tertiary", bg: "bg-tertiary/10" },
    { label: "Failing", count: failing, text: "text-error", bg: "bg-error/10" },
  ] as const;

  return (
    <div className="grid grid-cols-3 gap-4">
      {cards.map((card) => (
        <div
          key={card.label}
          className={`glass flex flex-col items-center justify-center gap-1 rounded-2xl border border-outline-variant px-4 py-5 ${card.bg}`}
        >
          <span className={`text-3xl font-bold ${card.text}`}>
            {card.count}
          </span>
          <span className="font-label text-xs uppercase tracking-wider text-on-surface-variant">
            {card.label}
          </span>
        </div>
      ))}
    </div>
  );
}
