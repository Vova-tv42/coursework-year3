import { isRouteErrorResponse, Link, useRouteError } from "react-router";

import { Button, buttonVariants } from "@/components/ui/button";
import {
  Card,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export function NotFoundPage() {
  return (
    <Card className="mx-auto max-w-sm">
      <CardHeader>
        <CardTitle>Page not found</CardTitle>
        <CardDescription>
          This page does not exist or you have no access to it.
        </CardDescription>
      </CardHeader>
      <CardFooter>
        <Link to="/" className={buttonVariants()}>
          Back to students
        </Link>
      </CardFooter>
    </Card>
  );
}

export function RouteError() {
  const error = useRouteError();
  const isNotFound = isRouteErrorResponse(error) && error.status === 404;
  if (isNotFound) return <NotFoundPage />;

  const message = error instanceof Error ? error.message : "Unexpected error";
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
  );
}
