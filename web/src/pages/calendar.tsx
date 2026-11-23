import { useMutation, useQuery } from "@tanstack/react-query";
import { parseISO } from "date-fns";
import { CalendarIcon } from "lucide-react";
import { useState } from "react";
import type { DateRange } from "react-day-picker";
import { toast } from "sonner";

import {
  api,
  calendarQuery,
  periodStreamLabels,
  queryClient,
  type CalendarPeriod,
  type PeriodKind,
} from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useCurrentUser } from "@/lib/hooks";
import { formatPeriodDates, toApiDate } from "@/lib/utils";

const periodKindLabels: Record<PeriodKind, string> = {
  work_start: "Work start",
  practice: "Practice",
};

// parseISO reads "2027-04-01" as local midnight, so the picker shows the same day
function toDateRange(period: CalendarPeriod): DateRange | undefined {
  if (period.startDate === null || period.endDate === null) return undefined;
  return { from: parseISO(period.startDate), to: parseISO(period.endDate) };
}

function PeriodDatesPicker({
  dates,
  onChange,
}: {
  dates: DateRange | undefined;
  onChange: (dates: DateRange | undefined) => void;
}) {
  const shownStartDate = dates?.from ? toApiDate(dates.from) : null;
  const shownEndDate = dates?.to ? toApiDate(dates.to) : shownStartDate;

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button variant="outline" className="justify-start font-normal" />
        }
      >
        <CalendarIcon />
        {formatPeriodDates(shownStartDate, shownEndDate)}
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="range"
          numberOfMonths={2}
          selected={dates}
          defaultMonth={dates?.from}
          onSelect={onChange}
        />
      </PopoverContent>
    </Popover>
  );
}

function EditablePeriodRow({ period }: { period: CalendarPeriod }) {
  const [name, setName] = useState(period.name);
  const [dates, setDates] = useState(() => toDateRange(period));

  const trimmedName = name.trim();
  const startDate = dates?.from ? toApiDate(dates.from) : null;
  // A range with only the first day picked is a one-day period
  const endDate = dates?.to ? toApiDate(dates.to) : startDate;
  const hasChanges =
    trimmedName !== period.name ||
    startDate !== period.startDate ||
    endDate !== period.endDate;

  const save = useMutation({
    mutationFn: () =>
      api.updateCalendarPeriod(period.id, {
        name: trimmedName,
        startDate,
        endDate,
      }),
    onSuccess: async (saved) => {
      queryClient.setQueryData(calendarQuery.queryKey, (periods) =>
        periods?.map((current) => (current.id === saved.id ? saved : current)),
      );
      await queryClient.invalidateQueries({ queryKey: ["analysis"] });
      await queryClient.invalidateQueries({ queryKey: ["students"] });
      toast.success("Period saved");
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <TableRow>
      <TableCell>
        <Input
          value={name}
          maxLength={100}
          aria-label="Period name"
          onChange={(event) => setName(event.target.value)}
        />
      </TableCell>
      <TableCell>{periodKindLabels[period.kind]}</TableCell>
      <TableCell>{periodStreamLabels[period.stream]}</TableCell>
      <TableCell>
        <PeriodDatesPicker dates={dates} onChange={setDates} />
      </TableCell>
      <TableCell>
        <Button
          disabled={!hasChanges || save.isPending}
          onClick={() => save.mutate()}
        >
          Save
        </Button>
      </TableCell>
    </TableRow>
  );
}

function ReadOnlyPeriodRow({ period }: { period: CalendarPeriod }) {
  return (
    <TableRow>
      <TableCell>{period.name}</TableCell>
      <TableCell>{periodKindLabels[period.kind]}</TableCell>
      <TableCell>{periodStreamLabels[period.stream]}</TableCell>
      <TableCell>
        {formatPeriodDates(period.startDate, period.endDate)}
      </TableCell>
    </TableRow>
  );
}

export function CalendarPage() {
  const user = useCurrentUser();
  const canEditCalendar =
    user.role === "coordinator" || user.role === "repo_manager";
  const periods = useQuery(calendarQuery);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Academic calendar</CardTitle>
        <CardDescription>
          Commits are compared with these dates by their day in the Europe/Kyiv
          time zone. A practice period without dates gives no signal.
          {!canEditCalendar &&
            " Only coordinators and repository managers can change it."}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {periods.isError && (
          <Alert variant="destructive">
            <AlertDescription>{periods.error.message}</AlertDescription>
          </Alert>
        )}
        {periods.isPending && (
          <p className="text-muted-foreground">Loading...</p>
        )}
        {periods.isSuccess && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Kind</TableHead>
                <TableHead>Stream</TableHead>
                <TableHead>Dates</TableHead>
                {canEditCalendar && <TableHead />}
              </TableRow>
            </TableHeader>
            <TableBody>
              {periods.data.map((period) =>
                canEditCalendar ? (
                  <EditablePeriodRow key={period.id} period={period} />
                ) : (
                  <ReadOnlyPeriodRow key={period.id} period={period} />
                ),
              )}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
