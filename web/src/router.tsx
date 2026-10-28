import { ArrowLeft } from 'lucide-react'
import {
  createBrowserRouter,
  createContext,
  data,
  isRouteErrorResponse,
  Link,
  redirect,
  useLoaderData,
  useRouteError,
  type LoaderFunctionArgs,
  type MiddlewareFunction,
} from 'react-router'

import {
  api,
  isApiErrorWithStatus,
  queryClient,
  streamLabels,
  studentQuery,
  type StudentOverview,
  type User,
} from '@/api/client'
import { Layout, signedInRouteId } from '@/components/layout'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { LoginPage } from '@/pages/login'
import { RegisterPage } from '@/pages/register'
import { StudentsPage } from '@/pages/students'

const userContext = createContext<User>()

const requireSignedInUser: MiddlewareFunction = async ({ context }) => {
  try {
    context.set(userContext, await api.me())
  } catch (error) {
    if (isApiErrorWithStatus(error, 401)) throw redirect('/login')
    throw error
  }
}

async function redirectIfSignedIn() {
  try {
    await api.me()
  } catch (error) {
    if (isApiErrorWithStatus(error, 401)) return null
    throw error
  }
  return redirect('/')
}

async function loadStudent({ params }: LoaderFunctionArgs) {
  try {
    return await queryClient.ensureQueryData(studentQuery(params.studentId ?? ''))
  } catch (error) {
    if (isApiErrorWithStatus(error, 404)) throw data(null, { status: 404 })
    throw error
  }
}

function NotFoundPage() {
  return (
    <Card className="mx-auto max-w-sm">
      <CardHeader>
        <CardTitle>Page not found</CardTitle>
        <CardDescription>This page does not exist or you have no access to it.</CardDescription>
      </CardHeader>
      <CardFooter>
        <Link to="/" className={buttonVariants()}>
          Back to students
        </Link>
      </CardFooter>
    </Card>
  )
}

function RouteError() {
  const error = useRouteError()
  const isNotFound = isRouteErrorResponse(error) && error.status === 404
  if (isNotFound) return <NotFoundPage />

  const message = error instanceof Error ? error.message : 'Unexpected error'
  return (
    <Card className="mx-auto my-6 max-w-sm">
      <CardHeader>
        <CardTitle>Something went wrong</CardTitle>
        <CardDescription>{message}</CardDescription>
      </CardHeader>
      <CardFooter>
        <Button variant="outline" onClick={() => window.location.reload()}>
          Reload
        </Button>
      </CardFooter>
    </Card>
  )
}

// Replaced by the repository page with analysis tabs later.
function StudentPage() {
  const student = useLoaderData() as StudentOverview

  return (
    <Card>
      <CardHeader>
        <CardTitle>{student.fullName}</CardTitle>
        <CardDescription>
          {streamLabels[student.stream]}, supervisor {student.supervisorName}
        </CardDescription>
        <CardAction>
          <Link to="/" className={buttonVariants({ variant: 'outline' })}>
            <ArrowLeft />
            Students
          </Link>
        </CardAction>
      </CardHeader>
      <CardContent>{student.repository.cloneUrl}</CardContent>
    </Card>
  )
}

export const router = createBrowserRouter([
  {
    hydrateFallbackElement: <div className="min-h-svh" />,
    children: [
      { path: '/login', loader: redirectIfSignedIn, element: <LoginPage /> },
      { path: '/register', loader: redirectIfSignedIn, element: <RegisterPage /> },
      {
        id: signedInRouteId,
        path: '/',
        middleware: [requireSignedInUser],
        loader: ({ context }) => context.get(userContext),
        element: <Layout />,
        errorElement: <RouteError />,
        children: [
          { index: true, element: <StudentsPage /> },
          {
            path: 'students/:studentId',
            loader: loadStudent,
            element: <StudentPage />,
            errorElement: <RouteError />,
          },
          { path: '*', element: <NotFoundPage /> },
        ],
      },
    ],
  },
])
