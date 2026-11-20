import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { ColorSchemeType } from "diff2html/lib-esm/types";
import { Diff2HtmlUI } from "diff2html/lib-esm/ui/js/diff2html-ui-base";

import "diff2html/bundles/css/diff2html.min.css";
import { ArrowLeft, ChevronDown, ChevronUp } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Link, useParams } from "react-router";

import {
  commitDiffQuery,
  commitQuery,
  repositoryQuery,
  type CommitDetails,
  type CommitFile,
} from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
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
import { useTheme } from "@/lib/hooks";
import { cn, firstLine } from "@/lib/utils";

const shortSHALength = 7;

const commitDateFormat = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeStyle: "medium",
  timeZone: "Europe/Kyiv",
});

export function CommitLink({
  repositoryId,
  sha,
}: {
  repositoryId: number;
  sha: string;
}) {
  return (
    <Link
      to={`/repos/${repositoryId}/commits/${sha}`}
      className={cn(buttonVariants({ variant: "link" }), "px-0")}
    >
      {sha.slice(0, shortSHALength)}
    </Link>
  );
}

function messageBody(message: string) {
  return message.split("\n").slice(1).join("\n").trim();
}

function TouchedFilesTable({ commit }: { commit: CommitDetails }) {
  if (commit.files.length === 0) {
    return (
      <p className="text-muted-foreground">
        {commit.isMerge
          ? "Merge commits are not split into files. The diff below is against the first parent."
          : "This commit does not change any files."}
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Path</TableHead>
          <TableHead>Added</TableHead>
          <TableHead>Deleted</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {commit.files.map((file) => (
          <TableRow key={file.path}>
            <TableCell>
              <div>{file.path}</div>
              {file.oldPath && (
                <div className="text-muted-foreground">
                  Renamed from {file.oldPath}
                </div>
              )}
            </TableCell>
            <LineCountCells file={file} />
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function LineCountCells({ file }: { file: CommitFile }) {
  if (file.isBinary) {
    return <TableCell colSpan={2}>Binary file</TableCell>;
  }

  return (
    <>
      <TableCell>{file.linesAdded}</TableCell>
      <TableCell>{file.linesDeleted}</TableCell>
    </>
  );
}

function FileCollapseButton({ fileHeader }: { fileHeader: HTMLElement }) {
  const [isCollapsed, setIsCollapsed] = useState(false);

  function toggleFileContent() {
    const fileContent = fileHeader
      .closest(".d2h-file-wrapper")
      ?.querySelector(".d2h-files-diff, .d2h-file-diff");
    fileContent?.classList.toggle("d2h-d-none", !isCollapsed);
    setIsCollapsed(!isCollapsed);
  }

  return (
    <Button
      variant="ghost"
      size="icon-xs"
      aria-label={isCollapsed ? "Expand file" : "Collapse file"}
      onClick={toggleFileContent}
    >
      {isCollapsed ? <ChevronDown /> : <ChevronUp />}
    </Button>
  );
}

// Diff2HtmlUI owns the diff markup, so the collapse buttons are rendered into its file headers with portals
function DiffContent({ diff }: { diff: string }) {
  const { theme } = useTheme();
  const containerRef = useRef<HTMLDivElement>(null);
  const [fileHeaders, setFileHeaders] = useState<HTMLElement[]>([]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    // The app theme is a class on <html>, so the diff follows it instead of the system setting
    const diffView = new Diff2HtmlUI(container, diff, {
      outputFormat: "side-by-side",
      drawFileList: false,
      highlight: false,
      fileContentToggle: false,
      colorScheme:
        theme === "dark" ? ColorSchemeType.DARK : ColorSchemeType.LIGHT,
    });
    diffView.draw();
    setFileHeaders(
      Array.from(container.querySelectorAll<HTMLElement>(".d2h-file-header")),
    );
  }, [diff, theme]);

  return (
    <>
      <div ref={containerRef} />
      {fileHeaders.map((fileHeader, index) =>
        createPortal(
          <FileCollapseButton fileHeader={fileHeader} />,
          fileHeader,
          index,
        ),
      )}
    </>
  );
}

function CommitDiffView({
  repositoryId,
  sha,
}: {
  repositoryId: number;
  sha: string;
}) {
  const diff = useQuery({
    ...commitDiffQuery(repositoryId, sha),
    staleTime: Infinity,
  });

  if (diff.isPending)
    return <p className="text-muted-foreground">Loading...</p>;
  if (diff.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{diff.error.message}</AlertDescription>
      </Alert>
    );
  }

  if (diff.data.diff === "") {
    return <p className="text-muted-foreground">The diff is empty.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      {diff.data.truncated && (
        <Alert>
          <AlertDescription>
            The diff is larger than 2 MB, only its beginning is shown.
          </AlertDescription>
        </Alert>
      )}
      <DiffContent diff={diff.data.diff} />
    </div>
  );
}

export function CommitPage() {
  const { repoId = "", sha = "" } = useParams();
  const repositoryId = Number(repoId);
  const { data: commit } = useSuspenseQuery(commitQuery(repositoryId, sha));
  const { data: repository } = useSuspenseQuery(repositoryQuery(repositoryId));
  const body = messageBody(commit.message);

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{firstLine(commit.message)}</CardTitle>
          <CardDescription>
            {commit.sha.slice(0, shortSHALength)} by {commit.authorName},{" "}
            {commitDateFormat.format(new Date(commit.authoredAt))}
          </CardDescription>
          <CardAction>
            <Link
              to={`/students/${repository.studentId}`}
              className={buttonVariants({ variant: "outline" })}
            >
              <ArrowLeft />
              Repository
            </Link>
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex gap-2">
            {commit.isMerge && <Badge variant="secondary">Merge</Badge>}
            <Badge variant="outline">
              +{commit.linesAdded} -{commit.linesDeleted}
            </Badge>
          </div>
          {body && <p className="whitespace-pre-wrap">{body}</p>}
          <p className="break-all text-muted-foreground">{commit.sha}</p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Touched files</CardTitle>
          <CardDescription>
            {commit.files.length} files changed in this commit.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <TouchedFilesTable commit={commit} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Diff</CardTitle>
        </CardHeader>
        <CardContent>
          <CommitDiffView repositoryId={repositoryId} sha={sha} />
        </CardContent>
      </Card>
    </div>
  );
}
