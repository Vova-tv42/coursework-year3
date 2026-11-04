import { useQuery } from "@tanstack/react-query";

import {
  commitSizesQuery,
  type CommitSizes as CommitSizeStats,
  type CommitSummary,
} from "@/api/client";
import { Chart, type ChartOption } from "@/components/chart";
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

const commitDateFormat = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Europe/Kyiv",
});

const lineCountFormat = new Intl.NumberFormat("en-GB", {
  maximumFractionDigits: 1,
});

const shortSHALength = 7;

function bucketChartOption(sizes: CommitSizeStats): ChartOption {
  return {
    tooltip: { trigger: "axis" },
    grid: { left: 56, right: 24, bottom: 48 },
    xAxis: {
      type: "category",
      name: "Lines",
      data: sizes.buckets.map((bucket) => bucket.label),
    },
    yAxis: { type: "value", name: "Commits", minInterval: 1 },
    series: [
      {
        name: "Commits",
        type: "bar",
        data: sizes.buckets.map((bucket) => bucket.count),
      },
    ],
  };
}

function firstLine(message: string) {
  return message.split("\n", 1)[0];
}

function LargeCommitsTable({ commits }: { commits: CommitSummary[] }) {
  if (commits.length === 0) {
    return (
      <p className="text-muted-foreground">
        No commits over 1000 changed lines.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Date</TableHead>
          <TableHead>SHA</TableHead>
          <TableHead>Message</TableHead>
          <TableHead>Author</TableHead>
          <TableHead>Added</TableHead>
          <TableHead>Deleted</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {commits.map((commit) => (
          <TableRow key={commit.sha}>
            <TableCell>
              {commitDateFormat.format(new Date(commit.authoredAt))}
            </TableCell>
            <TableCell>{commit.sha.slice(0, shortSHALength)}</TableCell>
            <TableCell>{firstLine(commit.message)}</TableCell>
            <TableCell>{commit.authorName}</TableCell>
            <TableCell>{commit.linesAdded}</TableCell>
            <TableCell>{commit.linesDeleted}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

export function CommitSizes({ repositoryId }: { repositoryId: number }) {
  const sizes = useQuery(commitSizesQuery(repositoryId));

  if (sizes.isPending)
    return <p className="text-muted-foreground">Loading...</p>;
  if (sizes.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{sizes.error.message}</AlertDescription>
      </Alert>
    );
  }

  if (sizes.data.nonMergeCommits === 0) {
    return (
      <p className="text-muted-foreground">
        The default branch has no commits except merges.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>Commit sizes</CardTitle>
          <CardDescription>
            {sizes.data.nonMergeCommits} commits without merges, mean{" "}
            {lineCountFormat.format(sizes.data.mean)} and median{" "}
            {lineCountFormat.format(sizes.data.median)} changed lines.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Chart option={bucketChartOption(sizes.data)} height={280} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Large commits</CardTitle>
          <CardDescription>
            Commits with more than 1000 changed lines.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <LargeCommitsTable commits={sizes.data.largeCommits} />
        </CardContent>
      </Card>
    </div>
  );
}
