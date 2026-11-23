import { QueryClient, queryOptions } from "@tanstack/react-query";

export type Role = "supervisor" | "coordinator" | "repo_manager";
export type Stream = "bachelor" | "master";
export type AnalysisStatus = "never" | "queued" | "running" | "done" | "failed";
export type PeriodStream = "all" | Stream;
export type PeriodKind = "work_start" | "practice";

export interface User {
  id: number;
  email: string;
  displayName: string;
  role: Role;
  stream?: Stream;
}

export interface RegisterRequest {
  email: string;
  password: string;
  displayName: string;
  role: Role;
  stream: Stream | null;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface StudentInput {
  fullName: string;
  stream: Stream;
  cloneUrl: string;
}

export interface HostingMetadata {
  available: boolean;
  defaultBranch?: string;
  pushedAt?: string;
}

export interface Repository {
  id: number;
  studentId: number;
  cloneUrl: string;
  defaultBranch: string | null;
  status: AnalysisStatus;
  statusReason: string | null;
  analysisStartedAt: string | null;
  analysisFinishedAt: string | null;
  lastCommitAt: string | null;
  hosting: HostingMetadata | null;
}

export interface StudentFlags {
  admitted: boolean | null;
  noPracticeActivity: boolean;
  rapidSeries: boolean;
}

export interface StudentOverview {
  id: number;
  fullName: string;
  stream: Stream;
  supervisorName: string;
  canEdit: boolean;
  repository: Repository;
  flags: StudentFlags;
}

export interface StudentFilters {
  stream: Stream | null;
  rapidSeries: boolean | null;
  admitted: boolean | null;
  noPracticeActivity: boolean | null;
}

export interface CommitSummary {
  sha: string;
  authorName: string;
  authorEmail: string;
  authoredAt: string;
  message: string;
  isMerge: boolean;
  linesAdded: number;
  linesDeleted: number;
}

export interface DayActivity {
  date: string;
  commits: number;
  linesChanged: number;
}

export interface RapidSeries {
  startedAt: string;
  endedAt: string;
  commits: CommitSummary[];
}

export interface CalendarPeriod {
  id: number;
  name: string;
  kind: PeriodKind;
  stream: PeriodStream;
  startDate: string | null;
  endDate: string | null;
}

export interface CalendarPeriodInput {
  name: string;
  startDate: string | null;
  endDate: string | null;
}

export interface PeriodActivity extends CalendarPeriod {
  commits: number;
  linesChanged: number;
  noActivitySignal: boolean;
}

export interface Activity {
  timeZone: string;
  days: DayActivity[];
  periods: PeriodActivity[];
  rapidSeries: RapidSeries[];
}

export interface CommitSizes {
  buckets: { label: string; count: number }[];
  mean: number;
  median: number;
  nonMergeCommits: number;
  largeCommits: CommitSummary[];
}

export type ChecklistItemKey =
  | "code"
  | "explanatory_note"
  | "presentation"
  | "readme_deployment";

export interface ChecklistItem {
  key: ChecklistItemKey;
  found: boolean;
  files: string[];
}

export interface Checklist {
  evaluated: boolean;
  admitted: boolean;
  items: ChecklistItem[];
}

export interface LineCounts {
  code: number;
  comments: number;
  docs: number;
  tests: number;
  config: number;
  total: number;
}

export interface CodeStats {
  lines: LineCounts;
  junk: {
    files: number;
    bytes: number;
    lines: number;
    samplePaths: string[];
  };
}

export interface FileModifications {
  path: string;
  modifications: number;
  linesAdded: number;
  linesDeleted: number;
  lastModifiedAt: string;
}

export interface CommitFile {
  path: string;
  oldPath: string | null;
  linesAdded: number;
  linesDeleted: number;
  isBinary: boolean;
}

export interface CommitDetails extends CommitSummary {
  files: CommitFile[];
}

export interface CommitDiff {
  diff: string;
  truncated: boolean;
}

export interface JunkPatterns {
  patterns: string[];
  isDefault: boolean;
}

interface ErrorResponse {
  code: string;
  message: string;
}

export const roleLabels: Record<Role, string> = {
  supervisor: "Supervisor",
  coordinator: "Coordinator",
  repo_manager: "Repository manager",
};

export const streamLabels: Record<Stream, string> = {
  bachelor: "Bachelor",
  master: "Master",
};

export const periodStreamLabels: Record<PeriodStream, string> = {
  all: "All",
  ...streamLabels,
};

export const statusLabels: Record<AnalysisStatus, string> = {
  never: "Not analyzed",
  queued: "Queued",
  running: "Running",
  done: "Done",
  failed: "Failed",
};

export const statusBadgeVariants: Record<
  AnalysisStatus,
  "outline" | "secondary" | "default" | "destructive"
> = {
  never: "outline",
  queued: "secondary",
  running: "secondary",
  done: "default",
  failed: "destructive",
};

export function isAnalysisInProgress(status: AnalysisStatus) {
  return status === "queued" || status === "running";
}

function studentsPath({
  stream,
  rapidSeries,
  admitted,
  noPracticeActivity,
}: StudentFilters) {
  const params = new URLSearchParams();
  if (stream) params.set("stream", stream);
  if (rapidSeries !== null) params.set("rapidSeries", String(rapidSeries));
  if (admitted !== null) params.set("admitted", String(admitted));
  if (noPracticeActivity !== null) {
    params.set("noPracticeActivity", String(noPracticeActivity));
  }

  const query = params.toString();
  return query ? `/students?${query}` : "/students";
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const hasBody = body !== undefined;
  const response = await fetch(`/api${path}`, {
    method,
    headers: hasBody ? { "Content-Type": "application/json" } : undefined,
    body: hasBody ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    const error = (await response
      .json()
      .catch(() => null)) as ErrorResponse | null;

    throw new ApiError(
      response.status,
      error?.code ?? "unknown",
      error?.message ?? response.statusText,
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export const api = {
  me: () => request<User>("GET", "/auth/me"),
  login: (input: LoginRequest) => request<User>("POST", "/auth/login", input),
  register: (input: RegisterRequest) =>
    request<User>("POST", "/auth/register", input),
  logout: () => request<void>("POST", "/auth/logout"),

  listStudents: (filters: StudentFilters) =>
    request<StudentOverview[]>("GET", studentsPath(filters)),
  getStudent: (studentId: string) =>
    request<StudentOverview>("GET", `/students/${studentId}`),
  createStudent: (input: StudentInput) =>
    request<StudentOverview>("POST", "/students", input),
  updateStudent: (studentId: number, input: StudentInput) =>
    request<StudentOverview>("PUT", `/students/${studentId}`, input),
  deleteStudent: (studentId: number) =>
    request<void>("DELETE", `/students/${studentId}`),

  getRepository: (repositoryId: number) =>
    request<Repository>("GET", `/repos/${repositoryId}`),
  analyzeRepository: (repositoryId: number) =>
    request<Repository>("POST", `/repos/${repositoryId}/analyze`),
  getActivity: (repositoryId: number) =>
    request<Activity>("GET", `/repos/${repositoryId}/activity`),
  getCommitSizes: (repositoryId: number) =>
    request<CommitSizes>("GET", `/repos/${repositoryId}/commit-sizes`),
  getChecklist: (repositoryId: number) =>
    request<Checklist>("GET", `/repos/${repositoryId}/checklist`),
  getCodeStats: (repositoryId: number) =>
    request<CodeStats>("GET", `/repos/${repositoryId}/code-stats`),
  getTopFiles: (repositoryId: number) =>
    request<FileModifications[]>("GET", `/repos/${repositoryId}/files`),
  getFileCommits: (repositoryId: number, path: string) =>
    request<CommitSummary[]>(
      "GET",
      `/repos/${repositoryId}/file-commits?${new URLSearchParams({ path })}`,
    ),
  getCommit: (repositoryId: number, sha: string) =>
    request<CommitDetails>("GET", `/repos/${repositoryId}/commits/${sha}`),
  getCommitDiff: (repositoryId: number, sha: string) =>
    request<CommitDiff>("GET", `/repos/${repositoryId}/commits/${sha}/diff`),

  listCalendarPeriods: () => request<CalendarPeriod[]>("GET", "/calendar"),
  updateCalendarPeriod: (periodId: number, input: CalendarPeriodInput) =>
    request<CalendarPeriod>("PUT", `/calendar/${periodId}`, input),

  getJunkPatterns: () => request<JunkPatterns>("GET", "/me/junk-patterns"),
  setJunkPatterns: (patterns: string[]) =>
    request<JunkPatterns>("PUT", "/me/junk-patterns", { patterns }),
  resetJunkPatterns: () => request<JunkPatterns>("DELETE", "/me/junk-patterns"),
};

export function isApiErrorWithStatus(error: unknown, status: number) {
  return error instanceof ApiError && error.status === status;
}

const maxQueryRetries = 2;

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        const isClientError = error instanceof ApiError && error.status < 500;
        return !isClientError && failureCount < maxQueryRetries;
      },
    },
  },
});

export const studentsQuery = (filters: StudentFilters) =>
  queryOptions({
    queryKey: ["students", filters],
    queryFn: () => api.listStudents(filters),
  });

export const studentQuery = (studentId: string) =>
  queryOptions({
    queryKey: ["student", studentId],
    queryFn: () => api.getStudent(studentId),
  });

export const repositoryQuery = (repositoryId: number) =>
  queryOptions({
    queryKey: ["repository", repositoryId],
    queryFn: () => api.getRepository(repositoryId),
  });

// Every analysis result shares this key prefix, so a finished analysis refreshes all of them at once
export const analysisResultsKey = (repositoryId: number) => [
  "analysis",
  repositoryId,
];

export const activityQuery = (repositoryId: number) =>
  queryOptions({
    queryKey: [...analysisResultsKey(repositoryId), "activity"],
    queryFn: () => api.getActivity(repositoryId),
  });

export const commitSizesQuery = (repositoryId: number) =>
  queryOptions({
    queryKey: [...analysisResultsKey(repositoryId), "commit-sizes"],
    queryFn: () => api.getCommitSizes(repositoryId),
  });

export const checklistQuery = (repositoryId: number) =>
  queryOptions({
    queryKey: [...analysisResultsKey(repositoryId), "checklist"],
    queryFn: () => api.getChecklist(repositoryId),
  });

export const codeStatsQuery = (repositoryId: number) =>
  queryOptions({
    queryKey: [...analysisResultsKey(repositoryId), "code-stats"],
    queryFn: () => api.getCodeStats(repositoryId),
  });

export const topFilesQuery = (repositoryId: number) =>
  queryOptions({
    queryKey: [...analysisResultsKey(repositoryId), "files"],
    queryFn: () => api.getTopFiles(repositoryId),
  });

export const fileCommitsQuery = (repositoryId: number, path: string) =>
  queryOptions({
    queryKey: [...analysisResultsKey(repositoryId), "file-commits", path],
    queryFn: () => api.getFileCommits(repositoryId, path),
  });

export const commitQuery = (repositoryId: number, sha: string) =>
  queryOptions({
    queryKey: [...analysisResultsKey(repositoryId), "commit", sha],
    queryFn: () => api.getCommit(repositoryId, sha),
  });

export const commitDiffQuery = (repositoryId: number, sha: string) =>
  queryOptions({
    queryKey: [...analysisResultsKey(repositoryId), "commit-diff", sha],
    queryFn: () => api.getCommitDiff(repositoryId, sha),
  });

export const junkPatternsQuery = queryOptions({
  queryKey: ["junk-patterns"],
  queryFn: api.getJunkPatterns,
});

export const calendarQuery = queryOptions({
  queryKey: ["calendar"],
  queryFn: api.listCalendarPeriods,
});
