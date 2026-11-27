import { format, formatDuration, intervalToDuration, parseISO } from "date-fns";

import type { CommitSizeRange, RapidSeriesSettings } from "@/api/client";

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

export function formatRange(range: CommitSizeRange) {
  return range.to === null ? `${range.from}+` : `${range.from}-${range.to}`;
}

function formatSeconds(seconds: number) {
  return formatDuration(intervalToDuration({ start: 0, end: seconds * 1000 }));
}

export function describeRapidSeriesRules(settings: RapidSeriesSettings) {
  const rules: string[] = [];
  if (settings.consecutive.enabled) {
    rules.push(
      `at least ${settings.consecutive.minCommits} commits, each at most ${formatSeconds(settings.consecutive.maxGapSeconds)} after the previous one`,
    );
  }
  if (settings.window.enabled) {
    rules.push(
      `at least ${settings.window.minCommits} commits within ${formatSeconds(settings.window.windowSeconds)}`,
    );
  }

  const sentence = rules.join(", or ");
  return `${sentence.charAt(0).toUpperCase()}${sentence.slice(1)}.`;
}
