import { useQuery } from "@tanstack/react-query";

import {
  activityQuery,
  type DayActivity,
  type RapidSeries,
} from "@/api/client";
import {
  Chart,
  type ChartInstance,
  type ChartOption,
} from "@/components/chart";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const seriesTimeFormat = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeStyle: "medium",
  timeZone: "Europe/Kyiv",
});

const shortSHALength = 7;
const dayMs = 24 * 60 * 60 * 1000;

// Days without commits are added as zeros, so the slider moves in whole calendar days
function withEmptyDays(days: DayActivity[]): DayActivity[] {
  const activityByDate = new Map(days.map((day) => [day.date, day]));
  const firstDayMs = Date.parse(days[0].date);
  const lastDayMs = Date.parse(days[days.length - 1].date);

  const allDays: DayActivity[] = [];
  for (
    let dayMsValue = firstDayMs;
    dayMsValue <= lastDayMs;
    dayMsValue += dayMs
  ) {
    const date = new Date(dayMsValue).toISOString().slice(0, 10);
    allDays.push(
      activityByDate.get(date) ?? { date, commits: 0, linesChanged: 0 },
    );
  }

  return allDays;
}

// Turns the API date "2026-09-01" into "01-09-26"
function formatChartDate(isoDate: string) {
  const [year, month, day] = isoDate.split("-");
  return `${day}-${month}-${year.slice(2)}`;
}

interface ZoomWindow {
  start: number;
  end: number;
}

interface CategoryAxis {
  data: string[];
}

// ECharts has no snapping, so on release the slider is moved to the nearest whole days
function snapZoomToDaysOnRelease(chart: ChartInstance) {
  chart.getZr().on("mouseup", () => {
    const option = chart.getOption();
    const [zoomWindow] = option.dataZoom as ZoomWindow[];
    const [dayAxis] = option.xAxis as CategoryAxis[];
    const lastDayIndex = dayAxis.data.length - 1;

    chart.dispatchAction({
      type: "dataZoom",
      startValue: Math.round((zoomWindow.start / 100) * lastDayIndex),
      endValue: Math.round((zoomWindow.end / 100) * lastDayIndex),
    });
  });
}

function activityChartOption(days: DayActivity[]): ChartOption {
  const allDays = withEmptyDays(days);
  const dates = allDays.map((day) => formatChartDate(day.date));

  return {
    tooltip: { trigger: "axis" },
    axisPointer: { link: [{ xAxisIndex: "all" }] },
    grid: [
      { top: 32, left: 64, right: 24, height: 160 },
      { top: 248, left: 64, right: 24, height: 160 },
    ],
    xAxis: [
      {
        type: "category",
        gridIndex: 0,
        data: dates,
        axisLabel: { show: false },
      },
      {
        type: "category",
        gridIndex: 1,
        data: dates,
        // The tooltip and the slider still show the year
        axisLabel: {
          formatter: (date: string) => date.slice(0, "dd-mm".length),
        },
      },
    ],
    yAxis: [
      { type: "value", gridIndex: 0, name: "Commits", minInterval: 1 },
      { type: "value", gridIndex: 1, name: "Changed lines", minInterval: 1 },
    ],
    dataZoom: [
      { type: "inside", xAxisIndex: [0, 1] },
      {
        type: "slider",
        xAxisIndex: [0, 1],
        brushSelect: false,
        // Without throttling the zoom state is already final when the release handler reads it
        throttle: 0,
        bottom: 16,
        left: 96,
        right: 96,
      },
    ],
    series: [
      {
        name: "Commits",
        type: "line",
        symbol: "circle",
        showSymbol: true,
        xAxisIndex: 0,
        yAxisIndex: 0,
        data: allDays.map((day) => day.commits),
      },
      {
        name: "Changed lines",
        type: "line",
        symbol: "circle",
        showSymbol: true,
        xAxisIndex: 1,
        yAxisIndex: 1,
        data: allDays.map((day) => day.linesChanged),
      },
    ],
  };
}

function RapidSeriesTable({ rapidSeries }: { rapidSeries: RapidSeries[] }) {
  if (rapidSeries.length === 0) {
    return <p className="text-muted-foreground">No rapid series found.</p>;
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Started</TableHead>
          <TableHead>Ended</TableHead>
          <TableHead>Commits</TableHead>
          <TableHead>SHAs</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rapidSeries.map((series) => (
          <TableRow key={series.startedAt}>
            <TableCell>
              {seriesTimeFormat.format(new Date(series.startedAt))}
            </TableCell>
            <TableCell>
              {seriesTimeFormat.format(new Date(series.endedAt))}
            </TableCell>
            <TableCell>{series.commits.length}</TableCell>
            <TableCell>
              {series.commits
                .map((commit) => commit.sha.slice(0, shortSHALength))
                .join(", ")}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export function ActivityChart({ repositoryId }: { repositoryId: number }) {
  const activity = useQuery(activityQuery(repositoryId));

  if (activity.isPending)
    return <p className="text-muted-foreground">Loading...</p>;
  if (activity.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{activity.error.message}</AlertDescription>
      </Alert>
    );
  }

  if (activity.data.days.length === 0) {
    return (
      <p className="text-muted-foreground">
        The default branch has no commits.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>Activity per day</CardTitle>
          <CardDescription>
            Days in the {activity.data.timeZone} time zone, by author date.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Chart
            option={activityChartOption(activity.data.days)}
            height={500}
            onChartReady={snapZoomToDaysOnRelease}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Rapid commit series</CardTitle>
          <CardDescription>
            Three or more commits at most a minute apart. This is a signal to
            look at, not a verdict.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <RapidSeriesTable rapidSeries={activity.data.rapidSeries} />
        </CardContent>
      </Card>
    </div>
  );
}
