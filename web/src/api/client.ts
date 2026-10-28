import { QueryClient, queryOptions } from "@tanstack/react-query";

export type Role = "supervisor" | "coordinator" | "repo_manager";
export type Stream = "bachelor" | "master";
export type AnalysisStatus = "never" | "queued" | "running" | "done" | "failed";

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
}

export interface StudentOverview {
  id: number;
  fullName: string;
  stream: Stream;
  supervisorName: string;
  canEdit: boolean;
  repository: Repository;
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

export const statusLabels: Record<AnalysisStatus, string> = {
  never: "Not analyzed",
  queued: "Queued",
  running: "Running",
  done: "Done",
  failed: "Failed",
};

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

  listStudents: (stream: Stream | null) =>
    request<StudentOverview[]>(
      "GET",
      stream ? `/students?stream=${stream}` : "/students",
    ),
  getStudent: (studentId: string) =>
    request<StudentOverview>("GET", `/students/${studentId}`),
  createStudent: (input: StudentInput) =>
    request<StudentOverview>("POST", "/students", input),
  updateStudent: (studentId: number, input: StudentInput) =>
    request<StudentOverview>("PUT", `/students/${studentId}`, input),
  deleteStudent: (studentId: number) =>
    request<void>("DELETE", `/students/${studentId}`),
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

export const studentsQuery = (stream: Stream | null) =>
  queryOptions({
    queryKey: ["students", stream ?? "all"],
    queryFn: () => api.listStudents(stream),
  });

export const studentQuery = (studentId: string) =>
  queryOptions({
    queryKey: ["student", studentId],
    queryFn: () => api.getStudent(studentId),
  });
