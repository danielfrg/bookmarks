// Built once: the locale can't change while the popup is open.
let timestampFormat: Intl.DateTimeFormat | undefined;

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
