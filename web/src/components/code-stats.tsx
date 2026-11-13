import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";

import { codeStatsQuery, type LineCounts } from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { buttonVariants } from "@/components/ui/button";
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
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const lineCategories: { key: keyof LineCounts; label: string }[] = [
  { key: "code", label: "Code" },
  { key: "comments", label: "Comments" },
  { key: "docs", label: "Documentation" },
  { key: "tests", label: "Tests" },
  { key: "config", label: "Configuration" },
];

const numberFormat = new Intl.NumberFormat("en-GB");

const sizeUnits = ["byte", "kilobyte", "megabyte", "gigabyte"];

function formatBytes(bytes: number) {
  let unitIndex = 0;
  let size = bytes;
  while (size >= 1024 && unitIndex < sizeUnits.length - 1) {
    size /= 1024;
    unitIndex++;
  }

  return new Intl.NumberFormat("en-GB", {
    style: "unit",
    unit: sizeUnits[unitIndex],
    unitDisplay: "short",
    maximumFractionDigits: 1,
  }).format(size);
}

export function CodeStats({ repositoryId }: { repositoryId: number }) {
  const codeStats = useQuery(codeStatsQuery(repositoryId));

  if (codeStats.isPending)
    return <p className="text-muted-foreground">Loading...</p>;
  if (codeStats.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{codeStats.error.message}</AlertDescription>
      </Alert>
    );
  }

  const { lines, junk } = codeStats.data;
  const hiddenSamplePaths = junk.files - junk.samplePaths.length;

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Lines of code</CardTitle>
          <CardDescription>
            Non-blank lines at the latest state of the default branch, junk
            excluded.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Category</TableHead>
                <TableHead>Lines</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {lineCategories.map((category) => (
                <TableRow key={category.key}>
                  <TableCell>{category.label}</TableCell>
                  <TableCell>
                    {numberFormat.format(lines[category.key])}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell>Total</TableCell>
                <TableCell>{numberFormat.format(lines.total)}</TableCell>
              </TableRow>
            </TableFooter>
          </Table>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Junk</CardTitle>
          <CardDescription>
            {numberFormat.format(junk.files)} files, {formatBytes(junk.bytes)},{" "}
            {numberFormat.format(junk.lines)} lines.
          </CardDescription>
          <CardAction>
            <Link
              to="/settings"
              className={buttonVariants({ variant: "link" })}
            >
              Edit patterns
            </Link>
          </CardAction>
        </CardHeader>
        <CardContent>
          {junk.files === 0 ? (
            <p className="text-muted-foreground">No junk files found.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Path</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {junk.samplePaths.map((path) => (
                  <TableRow key={path}>
                    <TableCell className="break-all whitespace-normal">
                      {path}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              {hiddenSamplePaths > 0 && (
                <TableFooter>
                  <TableRow>
                    <TableCell className="text-muted-foreground">
                      And {numberFormat.format(hiddenSamplePaths)} more files
                    </TableCell>
                  </TableRow>
                </TableFooter>
              )}
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
