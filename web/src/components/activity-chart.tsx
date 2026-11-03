import { useQuery } from '@tanstack/react-query'

import { activityQuery, type DayActivity, type RapidSeries } from '@/api/client'
import { Chart, type ChartOption } from '@/components/chart'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const seriesTimeFormat = new Intl.DateTimeFormat('en-GB', {
  dateStyle: 'medium',
  timeStyle: 'medium',
  timeZone: 'Europe/Kyiv',
})

const shortSHALength = 7
const dayMs = 24 * 60 * 60 * 1000

function activityChartOption(days: DayActivity[]): ChartOption {
  return {
    tooltip: { trigger: 'axis' },
    legend: { top: 0 },
    grid: { top: 56, left: 56, right: 72, bottom: 80 },
    xAxis: { type: 'time', minInterval: dayMs },
    yAxis: [
      { type: 'value', name: 'Commits', minInterval: 1 },
      { type: 'value', name: 'Changed lines' },
    ],
    dataZoom: [{ type: 'inside' }, { type: 'slider' }],
    series: [
      { name: 'Commits', type: 'bar', data: days.map((day) => [day.date, day.commits]) },
      { name: 'Changed lines', type: 'bar', yAxisIndex: 1, data: days.map((day) => [day.date, day.linesChanged]) },
    ],
  }
}

function RapidSeriesTable({ rapidSeries }: { rapidSeries: RapidSeries[] }) {
  if (rapidSeries.length === 0) {
    return <p className="text-muted-foreground">No rapid series found.</p>
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
            <TableCell>{seriesTimeFormat.format(new Date(series.startedAt))}</TableCell>
            <TableCell>{seriesTimeFormat.format(new Date(series.endedAt))}</TableCell>
            <TableCell>{series.commits.length}</TableCell>
            <TableCell>
              {series.commits.map((commit) => commit.sha.slice(0, shortSHALength)).join(', ')}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

export function ActivityChart({ repositoryId }: { repositoryId: number }) {
  const activity = useQuery(activityQuery(repositoryId))

  if (activity.isPending) return <p className="text-muted-foreground">Loading...</p>
  if (activity.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{activity.error.message}</AlertDescription>
      </Alert>
    )
  }

  if (activity.data.days.length === 0) {
    return <p className="text-muted-foreground">The default branch has no commits.</p>
  }

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>Commits per day</CardTitle>
          <CardDescription>Days in the {activity.data.timeZone} time zone, by author date.</CardDescription>
        </CardHeader>
        <CardContent>
          <Chart option={activityChartOption(activity.data.days)} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Rapid commit series</CardTitle>
          <CardDescription>
            Three or more commits at most a minute apart. This is a signal to look at, not a verdict.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <RapidSeriesTable rapidSeries={activity.data.rapidSeries} />
        </CardContent>
      </Card>
    </div>
  )
}
