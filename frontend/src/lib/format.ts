const MINUTE = 60;
const HOUR = 3600;
const DAY = 86400;
const WEEK = 604800;

// Formatters are built once: these run per list row and the locale can't change mid-page.
let monthDayFormat: Intl.DateTimeFormat | undefined;
let timestampFormat: Intl.DateTimeFormat | undefined;

/** An ISO timestamp as "just now", "5m ago", "3d ago", then "Mar 4". */
export function formatRelativeTime(isoString: string | null | undefined, now = Date.now()): string {
  if (!isoString) return "—";
  const date = new Date(isoString);
  const diffSec = Math.floor((now - date.getTime()) / 1000);
  if (diffSec < MINUTE) return "just now";
  if (diffSec < HOUR) return `${Math.floor(diffSec / MINUTE)}m ago`;
  if (diffSec < DAY) return `${Math.floor(diffSec / HOUR)}h ago`;
  if (diffSec < WEEK) return `${Math.floor(diffSec / DAY)}d ago`;
  monthDayFormat ??= new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" });
  return monthDayFormat.format(date);
}

/** An ISO timestamp as a short date and time. */
export function formatTimestamp(isoString: string | null | undefined): string {
  if (!isoString) return "—";
  timestampFormat ??= new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
  return timestampFormat.format(new Date(isoString));
}
