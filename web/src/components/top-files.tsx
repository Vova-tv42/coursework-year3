import { useQuery } from "@tanstack/react-query";
import {
  createColumnHelper,
  createSortedRowModel,
  rowSortingFeature,
  sortFns,
  tableFeatures,
  useTable,
} from "@tanstack/react-table";
import { useMemo, useState } from "react";

import {
  fileCommitsQuery,
  topFilesQuery,
  type FileModifications,
} from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { firstLine } from "@/lib/utils";
import { CommitLink } from "@/pages/commit";
import { SortIcon } from "@/pages/students";

// A committed dependency folder can add thousands of rows, so only the top of the sorted list is rendered
const maxShownFiles = 200;

const dateFormat = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeZone: "Europe/Kyiv",
});

const dateTimeFormat = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Europe/Kyiv",
});

const features = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns,
});
const columnHelper = createColumnHelper<typeof features, FileModifications>();

function createColumns(onSelectFile: (path: string) => void) {
  return columnHelper.columns([
    columnHelper.accessor("path", {
      header: "Path",
      cell: (info) => (
        <Button
          variant="link"
          className="h-auto px-0 text-left break-all whitespace-normal"
          onClick={() => onSelectFile(info.getValue())}
        >
          {info.getValue()}
        </Button>
      ),
    }),
    columnHelper.accessor("modifications", { header: "Modifications" }),
    columnHelper.accessor("lastModifiedAt", {
      header: "Last modified",
      cell: (info) => dateFormat.format(new Date(info.getValue())),
    }),
  ]);
}

function FileCommitsSheet({
  repositoryId,
  path,
  onClose,
}: {
  repositoryId: number;
  path: string | null;
  onClose: () => void;
}) {
  const commits = useQuery({
    ...fileCommitsQuery(repositoryId, path ?? ""),
    enabled: path !== null,
  });

  return (
    <Sheet open={path !== null} onOpenChange={(open) => !open && onClose()}>
      <SheetContent className="data-[side=right]:sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle className="break-all">{path}</SheetTitle>
          <SheetDescription>Commits that changed this file.</SheetDescription>
        </SheetHeader>
        <div className="overflow-y-auto px-4 pb-4">
          {commits.isPending && (
            <p className="text-muted-foreground">Loading...</p>
          )}
          {commits.isError && (
            <Alert variant="destructive">
              <AlertDescription>{commits.error.message}</AlertDescription>
            </Alert>
          )}
          {commits.isSuccess && (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Date</TableHead>
                  <TableHead>SHA</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Author</TableHead>
                  <TableHead>Size</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {commits.data.map((commit) => (
                  <TableRow key={commit.sha}>
                    <TableCell>
                      {dateTimeFormat.format(new Date(commit.authoredAt))}
                    </TableCell>
                    <TableCell>
                      <CommitLink
                        repositoryId={repositoryId}
                        sha={commit.sha}
                      />
                    </TableCell>
                    <TableCell className="whitespace-normal">
                      {firstLine(commit.message)}
                    </TableCell>
                    <TableCell>{commit.authorName}</TableCell>
                    <TableCell>
                      +{commit.linesAdded} -{commit.linesDeleted}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

export function TopFiles({ repositoryId }: { repositoryId: number }) {
  const files = useQuery(topFilesQuery(repositoryId));
  const [pathFilter, setPathFilter] = useState("");
  const [selectedPath, setSelectedPath] = useState<string | null>(null);

  const columns = useMemo(() => createColumns(setSelectedPath), []);
  const normalizedFilter = pathFilter.trim().toLowerCase();
  const filteredFiles = useMemo(
    () =>
      (files.data ?? []).filter((file) =>
        file.path.toLowerCase().includes(normalizedFilter),
      ),
    [files.data, normalizedFilter],
  );
  const table = useTable({ features, columns, data: filteredFiles });

  if (files.isPending)
    return <p className="text-muted-foreground">Loading...</p>;
  if (files.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{files.error.message}</AlertDescription>
      </Alert>
    );
  }

  if (files.data.length === 0) {
    return (
      <p className="text-muted-foreground">
        The default branch has no file changes.
      </p>
    );
  }

  const sortedRows = table.getRowModel().rows;
  const shownRows = sortedRows.slice(0, maxShownFiles);
  const hiddenRowCount = sortedRows.length - shownRows.length;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Files in repository</CardTitle>
        <CardAction>
          <Input
            aria-label="Path filter"
            placeholder="Filter by path"
            value={pathFilter}
            onChange={(event) => setPathFilter(event.target.value)}
          />
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead key={header.id}>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={header.column.getToggleSortingHandler()}
                    >
                      <table.FlexRender header={header} />
                      <SortIcon direction={header.column.getIsSorted()} />
                    </Button>
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {shownRows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className="text-muted-foreground"
                >
                  No files match the filter.
                </TableCell>
              </TableRow>
            ) : (
              shownRows.map((row) => (
                <TableRow key={row.id}>
                  {row.getAllCells().map((cell) => (
                    <TableCell key={cell.id}>
                      <table.FlexRender cell={cell} />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        {hiddenRowCount > 0 && (
          <p className="text-muted-foreground">
            And {hiddenRowCount} more files. Use the filter to find them.
          </p>
        )}
      </CardContent>
      <FileCommitsSheet
        repositoryId={repositoryId}
        path={selectedPath}
        onClose={() => setSelectedPath(null)}
      />
    </Card>
  );
}
