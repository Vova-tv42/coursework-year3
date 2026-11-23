import { format, parseISO } from "date-fns";

export { cn } from "cn";

export function firstLine(message: string) {
  return message.split("\n", 1)[0];
}

const shownDateFormat = "d MMM yyyy";

export function toApiDate(date: Date) {
  return format(date, "yyyy-MM-dd");
}

export function formatPeriodDates(
  startDate: string | null,
  endDate: string | null,
) {
  if (startDate === null || endDate === null) return "Not set";

  const shownStart = format(parseISO(startDate), shownDateFormat);
  if (startDate === endDate) return shownStart;

  return `${shownStart} - ${format(parseISO(endDate), shownDateFormat)}`;
}
