import { useQuery } from "@tanstack/react-query";

import {
  activityQuery,
  settingQuery,
  type DayActivity,
  type PeriodActivity,
  type RapidSeries,
} from "@/api/client";
import {
  Chart,
  type ChartInstance,
  type ChartOption,
} from "@/components/chart";
import { RapidSeriesDialog } from "@/components/settings-dialogs";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardAction,
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
import { describeRapidSeriesRules, formatPeriodDates } from "@/lib/utils";
import { CommitLink } from "@/pages/commit";

const seriesTimeFormat = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeStyle: "medium",
  timeZone: "Europe/Kyiv",
});

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

// The border separates periods that follow each other; its color comes from the series
const periodBandStyle = { borderWidth: 1, borderType: "dashed" };
const periodLabelTop = 4;
const periodLabelGap = 8;
// The default ECharts label font, used to measure label widths
const periodLabelFont = "12px sans-serif";
const gridLeft = 64;
const gridRight = 24;

// Periods are cut to the days on the chart; ISO dates compare correctly as strings
function periodBands(periods: PeriodActivity[], allDays: DayActivity[]) {
  const firstDate = allDays[0].date;
  const lastDate = allDays[allDays.length - 1].date;

  const visiblePeriods = periods.flatMap((period) => {
    if (period.startDate === null || period.endDate === null) return [];

    const bandStart =
      period.startDate > firstDate ? period.startDate : firstDate;
    const bandEnd = period.endDate < lastDate ? period.endDate : lastDate;
    // ECharts draws a band between day centers, so a single day would have no width
    const isVisibleBand = bandStart < bandEnd;
    if (!isVisibleBand) return [];

    return [{ name: period.name, bandStart, bandEnd }];
  });

  // A position array is not an "inside" position, so ECharts keeps the theme text color
  return visiblePeriods.map((period) => [
    {
      name: period.name,
      xAxis: formatChartDate(period.bandStart),
      label: { position: ["50%", periodLabelTop], align: "center" },
    },
    { xAxis: formatChartDate(period.bandEnd) },
  ]);
}

interface BandPoint {
  name?: string;
  xAxis: string;
  label?: { show?: boolean };
}

interface SeriesWithBands {
  markArea?: { data: [BandPoint, BandPoint][] };
}

let labelMeasureContext: CanvasRenderingContext2D | null = null;

function measureLabelWidth(text: string) {
  labelMeasureContext ??= document.createElement("canvas").getContext("2d");
  if (!labelMeasureContext) return 0;

  labelMeasureContext.font = periodLabelFont;
  return labelMeasureContext.measureText(text).width;
}

// All labels share one row, so a label that would cover an earlier period's label is hidden for the current zoom
function hideOverlappingPeriodLabels(chart: ChartInstance) {
  const [commitsSeries] = chart.getOption().series as SeriesWithBands[];
  const bands = commitsSeries.markArea?.data ?? [];
  const gridRightEdge = chart.getWidth() - gridRight;
  const toPixel = (date: string) =>
    chart.convertToPixel({ xAxisIndex: 0 }, date) as number;
  const shownLabels: { left: number; right: number }[] = [];

  const updatedBands = bands.map(([start, end]): [BandPoint, BandPoint] => {
    // ECharts centers the label in the part of the band that is inside the grid
    const visibleLeft = Math.max(toPixel(start.xAxis), gridLeft);
    const visibleRight = Math.min(toPixel(end.xAxis), gridRightEdge);
    const center = (visibleLeft + visibleRight) / 2;
    const halfWidth = measureLabelWidth(start.name ?? "") / 2 + periodLabelGap;
    const label = { left: center - halfWidth, right: center + halfWidth };

    const isBandVisible = visibleLeft < visibleRight;
    const coversShownLabel = shownLabels.some(
      (shown) => label.left < shown.right && shown.left < label.right,
    );
    const isLabelShown = isBandVisible && !coversShownLabel;
    if (isLabelShown) shownLabels.push(label);

    return [{ ...start, label: { ...start.label, show: isLabelShown } }, end];
  });

  // Updating only on a change stops the loop, because setOption fires "finished" again
  const isVisibilityChanged = updatedBands.some(
    ([start], index) =>
      start.label?.show !== (bands[index][0].label?.show ?? true),
  );
  if (!isVisibilityChanged) return;

  chart.setOption({ series: [{ markArea: { data: updatedBands } }] });
}

function setUpActivityChart(chart: ChartInstance) {
  snapZoomToDaysOnRelease(chart);
  // "finished" follows every render: zoom, resize and new data
  chart.on("finished", () => hideOverlappingPeriodLabels(chart));
}

function activityChartOption(
  days: DayActivity[],
  periods: PeriodActivity[],
): ChartOption {
  const allDays = withEmptyDays(days);
  const dates = allDays.map((day) => formatChartDate(day.date));
  const bands = periodBands(periods, allDays);

  return {
    tooltip: { trigger: "axis" },
    axisPointer: { link: [{ xAxisIndex: "all" }] },
    grid: [
      { top: 32, left: gridLeft, right: gridRight, height: 160 },
      { top: 248, left: gridLeft, right: gridRight, height: 160 },
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
        markArea: { data: bands, itemStyle: periodBandStyle },
      },
      {
        name: "Changed lines",
        type: "line",
        symbol: "circle",
        showSymbol: true,
        xAxisIndex: 1,
        yAxisIndex: 1,
        data: allDays.map((day) => day.linesChanged),
        markArea: {
          data: bands,
          itemStyle: periodBandStyle,
          label: { show: false },
        },
      },
    ],
  };
}

function PeriodsTable({ periods }: { periods: PeriodActivity[] }) {
  if (periods.length === 0) {
    return <p className="text-muted-foreground">No calendar periods.</p>;
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Period</TableHead>
          <TableHead>Dates</TableHead>
          <TableHead>Commits</TableHead>
          <TableHead>Changed lines</TableHead>
          <TableHead>Signals</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {periods.map((period) => (
          <TableRow key={period.id}>
            <TableCell>{period.name}</TableCell>
            <TableCell>
              {formatPeriodDates(period.startDate, period.endDate)}
            </TableCell>
            <TableCell>{period.commits}</TableCell>
            <TableCell>{period.linesChanged}</TableCell>
            <TableCell>
              {period.noActivitySignal && (
                <Badge variant="destructive">No practice activity</Badge>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function RapidSeriesTable({
  repositoryId,
  rapidSeries,
}: {
  repositoryId: number;
  rapidSeries: RapidSeries[];
}) {
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
              <div className="flex flex-wrap gap-x-3">
                {series.commits.map((commit) => (
                  <CommitLink
                    key={commit.sha}
                    repositoryId={repositoryId}
                    sha={commit.sha}
                  />
                ))}
              </div>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export function ActivityChart({ repositoryId }: { repositoryId: number }) {
  const activity = useQuery(activityQuery(repositoryId));
  const rapidSeriesSettings = useQuery(settingQuery("rapid-series"));

  if (activity.isPending)
    return <p className="text-muted-foreground">Loading...</p>;
  if (activity.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{activity.error.message}</AlertDescription>
      </Alert>
    );
  }

  const hasCommits = activity.data.days.length > 0;

  return (
    <div className="flex flex-col gap-4">
      {hasCommits ? (
        <Card>
          <CardHeader>
            <CardTitle>Activity per day</CardTitle>
            <CardDescription>
              Days in the {activity.data.timeZone} time zone, by author date.
              Shaded areas are calendar periods.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Chart
              option={activityChartOption(
                activity.data.days,
                activity.data.periods,
              )}
              height={500}
              onChartReady={setUpActivityChart}
            />
          </CardContent>
        </Card>
      ) : (
        <p className="text-muted-foreground">
          The default branch has no commits.
        </p>
      )}
      <Card>
        <CardHeader>
          <CardTitle>Calendar periods</CardTitle>
          <CardDescription>
            Commits and changed lines inside each period of the student's
            stream.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <PeriodsTable periods={activity.data.periods} />
        </CardContent>
      </Card>
      {hasCommits && (
        <Card>
          <CardHeader>
            <CardTitle>Rapid commit series</CardTitle>
            <CardDescription>
              {rapidSeriesSettings.isSuccess &&
                `${describeRapidSeriesRules(rapidSeriesSettings.data.value)} `}
              This is a signal to look at, not a verdict.
            </CardDescription>
            <CardAction>
              <RapidSeriesDialog />
            </CardAction>
          </CardHeader>
          <CardContent>
            <RapidSeriesTable
              repositoryId={repositoryId}
              rapidSeries={activity.data.rapidSeries}
            />
          </CardContent>
        </Card>
      )}
    </div>
  );
}
