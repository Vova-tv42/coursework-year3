import { type SubmitEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router'

import { api, queryClient } from '@/api/client'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

export function LoginPage() {
  const navigate = useNavigate()

  const signIn = useMutation({
    mutationFn: api.login,
    onSuccess: async () => {
      queryClient.clear()
      await navigate('/')
    },
  })

  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    signIn.mutate({
      email: String(form.get('email')),
      password: String(form.get('password')),
    })
  }

  return (
    <div className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Sign in to Commit Radar</CardTitle>
          <CardDescription>Enter your email and password.</CardDescription>
        </CardHeader>
        <CardContent>
          <form id="login-form" onSubmit={handleSubmit}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="email">Email</FieldLabel>
                <Input id="email" name="email" type="email" autoComplete="email" required autoFocus />
              </Field>
              <Field>
                <FieldLabel htmlFor="password">Password</FieldLabel>
                <Input id="password" name="password" type="password" autoComplete="current-password" required />
              </Field>
              <FieldError>{signIn.error?.message}</FieldError>
            </FieldGroup>
          </form>
        </CardContent>
        <CardFooter className="flex flex-col gap-2">
          <Button type="submit" form="login-form" className="w-full" disabled={signIn.isPending}>
            Sign in
          </Button>
          <Link to="/register" className={buttonVariants({ variant: 'link' })}>
            Create an account
          </Link>
        </CardFooter>
      </Card>
    </div>
  )
}
