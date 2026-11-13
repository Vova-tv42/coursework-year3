import {
  createBrowserRouter,
  createContext,
  data,
  redirect,
  type LoaderFunctionArgs,
  type MiddlewareFunction,
} from "react-router";

import {
  api,
  isApiErrorWithStatus,
  queryClient,
  studentQuery,
  type User,
} from "@/api/client";
import { Layout } from "@/components/layout";
import { signedInRouteId } from "@/lib/hooks";
import { NotFoundPage, RouteError } from "@/pages/errors";
import { LoginPage } from "@/pages/login";
import { RegisterPage } from "@/pages/register";
import { SettingsPage } from "@/pages/settings";
import { StudentPage } from "@/pages/student";
import { StudentsPage } from "@/pages/students";

const userContext = createContext<User>();

const requireSignedInUser: MiddlewareFunction = async ({ context }) => {
  try {
    context.set(userContext, await api.me());
  } catch (error) {
    if (isApiErrorWithStatus(error, 401)) throw redirect("/login");
    throw error;
  }
};

async function redirectIfSignedIn() {
  try {
    await api.me();
  } catch (error) {
    if (isApiErrorWithStatus(error, 401)) return null;
    throw error;
  }
  return redirect("/");
}

async function loadStudent({ params }: LoaderFunctionArgs) {
  try {
    return await queryClient.ensureQueryData(
      studentQuery(params.studentId ?? ""),
    );
  } catch (error) {
    if (isApiErrorWithStatus(error, 404)) throw data(null, { status: 404 });
    throw error;
  }
}

export const router = createBrowserRouter([
  {
    hydrateFallbackElement: <div className="min-h-svh" />,
    children: [
      { path: "/login", loader: redirectIfSignedIn, element: <LoginPage /> },
      {
        path: "/register",
        loader: redirectIfSignedIn,
        element: <RegisterPage />,
      },
      {
        id: signedInRouteId,
        path: "/",
        middleware: [requireSignedInUser],
        loader: ({ context }) => context.get(userContext),
        element: <Layout />,
        errorElement: <RouteError />,
        children: [
          { index: true, element: <StudentsPage /> },
          {
            path: "students/:studentId",
            loader: loadStudent,
            element: <StudentPage />,
            errorElement: <RouteError />,
          },
          { path: "settings", element: <SettingsPage /> },
          { path: "*", element: <NotFoundPage /> },
        ],
      },
    ],
  },
]);
