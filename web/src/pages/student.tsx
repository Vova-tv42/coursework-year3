import { useEffect, useRef } from "react";
import { useMutation, useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { ArrowLeft, RefreshCw } from "lucide-react";
import { Link, useParams } from "react-router";
import { toast } from "sonner";

import {
  analysisResultsKey,
  api,
  isAnalysisInProgress,
  queryClient,
  repositoryQuery,
  statusBadgeVariants,
  statusLabels,
  streamLabels,
  studentQuery,
  type Repository,
} from "@/api/client";
import { ActivityChart } from "@/components/activity-chart";
import { Checklist } from "@/components/checklist";
import { CommitSizes } from "@/components/commit-sizes";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const statusPollIntervalMs = 2000;

const dateTimeFormat = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Europe/Kyiv",
});

function formatDateTime(value: string | null | undefined) {
  return value ? dateTimeFormat.format(new Date(value)) : "-";
}

function hostingDescription(repository: Repository) {
  const hosting = repository.hosting;
  if (!hosting?.available) return "Unavailable";

  const lastPush = hosting.pushedAt
    ? `, last push ${formatDateTime(hosting.pushedAt)}`
    : "";
  return `GitHub, default branch ${hosting.defaultBranch ?? "-"}${lastPush}`;
}

async function refreshAfterAnalysis(studentId: string, repositoryId: number) {
  await Promise.all([
    queryClient.invalidateQueries({
      queryKey: analysisResultsKey(repositoryId),
    }),
    queryClient.invalidateQueries({ queryKey: ["students"] }),
    queryClient.invalidateQueries({ queryKey: ["student", studentId] }),
  ]);
}

function RepositoryDetails({ repository }: { repository: Repository }) {
  const details = [
    { label: "Clone URL", value: repository.cloneUrl },
    { label: "Default branch", value: repository.defaultBranch ?? "-" },
    { label: "Hosting", value: hostingDescription(repository) },
    { label: "Last commit", value: formatDateTime(repository.lastCommitAt) },
    {
      label: "Last analysis",
      value: formatDateTime(repository.analysisFinishedAt),
    },
  ];

  return (
    <dl className="grid gap-2 sm:grid-cols-[max-content_1fr] sm:gap-x-6">
      {details.map((detail) => (
        <div key={detail.label} className="contents">
          <dt className="text-muted-foreground">{detail.label}</dt>
          <dd className="break-all">{detail.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function AnalysisResults({ repository }: { repository: Repository }) {
  const isInProgress = isAnalysisInProgress(repository.status);
  const hasNoResults =
    repository.status !== "done" && repository.lastCommitAt === null;

  if (hasNoResults) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>No analysis results yet</CardTitle>
          <CardDescription>
            {isInProgress
              ? "The repository is being analyzed. Results appear here when the analysis finishes."
              : "Press Analyze to read the history of the default branch."}
          </CardDescription>
        </CardHeader>
      </Card>
    );
  }

  return (
    <Tabs defaultValue="activity">
      <TabsList>
        <TabsTrigger value="activity">Activity</TabsTrigger>
        <TabsTrigger value="commit-sizes">Commit sizes</TabsTrigger>
        <TabsTrigger value="checklist">Checklist</TabsTrigger>
      </TabsList>
      <TabsContent value="activity">
        <ActivityChart repositoryId={repository.id} />
      </TabsContent>
      <TabsContent value="commit-sizes">
        <CommitSizes repositoryId={repository.id} />
      </TabsContent>
      <TabsContent value="checklist">
        <Checklist repositoryId={repository.id} />
      </TabsContent>
    </Tabs>
  );
}

export function StudentPage() {
  const { studentId = "" } = useParams();
  const { data: student } = useSuspenseQuery(studentQuery(studentId));
  const repositoryId = student.repository.id;

  const { data: repository } = useQuery({
    ...repositoryQuery(repositoryId),
    initialData: student.repository,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && isAnalysisInProgress(status)
        ? statusPollIntervalMs
        : false;
    },
  });

  const previousStatus = useRef(repository.status);
  useEffect(() => {
    const hasAnalysisFinished =
      isAnalysisInProgress(previousStatus.current) &&
      !isAnalysisInProgress(repository.status);
    previousStatus.current = repository.status;
    if (hasAnalysisFinished) void refreshAfterAnalysis(studentId, repositoryId);
  }, [repository.status, studentId, repositoryId]);

  const analyze = useMutation({
    mutationFn: () => api.analyzeRepository(repositoryId),
    onSuccess: (queuedRepository) =>
      queryClient.setQueryData(
        repositoryQuery(repositoryId).queryKey,
        queuedRepository,
      ),
    onError: (error) => toast.error(error.message),
  });

  const isAnalyzeDisabled =
    analyze.isPending || isAnalysisInProgress(repository.status);

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{student.fullName}</CardTitle>
          <CardDescription>
            {streamLabels[student.stream]}, supervisor {student.supervisorName}
          </CardDescription>
          <CardAction className="flex gap-2">
            <Link to="/" className={buttonVariants({ variant: "outline" })}>
              <ArrowLeft />
              Students
            </Link>
            <Button
              disabled={isAnalyzeDisabled}
              onClick={() => analyze.mutate()}
            >
              <RefreshCw />
              Analyze
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div>
            <Badge variant={statusBadgeVariants[repository.status]}>
              {statusLabels[repository.status]}
            </Badge>
          </div>
          {repository.status === "failed" && (
            <Alert variant="destructive">
              <AlertTitle>Analysis failed</AlertTitle>
              <AlertDescription>
                {repository.statusReason ?? "Unknown reason"}
              </AlertDescription>
            </Alert>
          )}
          <RepositoryDetails repository={repository} />
        </CardContent>
      </Card>
      <AnalysisResults repository={repository} />
    </div>
  );
}
