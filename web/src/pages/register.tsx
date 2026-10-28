import { useState, type SubmitEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router'

import { api, isApiErrorWithStatus, queryClient, roleLabels, streamLabels, type Role, type Stream } from '@/api/client'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const roleItems = Object.entries(roleLabels).map(([value, label]) => ({ value: value as Role, label }))
const streamItems = Object.entries(streamLabels).map(([value, label]) => ({ value: value as Stream, label }))

export function RegisterPage() {
  const navigate = useNavigate()
  const [role, setRole] = useState<Role>('supervisor')
  const [stream, setStream] = useState<Stream>('bachelor')
  const isCoordinator = role === 'coordinator'

  const register = useMutation({
    mutationFn: api.register,
    onSuccess: async () => {
      queryClient.clear()
      await navigate('/')
    },
  })
  const isEmailTaken = isApiErrorWithStatus(register.error, 409)
  const hasOtherError = register.error !== null && !isEmailTaken

  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    register.mutate({
      email: String(form.get('email')),
      displayName: String(form.get('displayName')),
      password: String(form.get('password')),
      role,
      stream: isCoordinator ? stream : null,
    })
  }

  return (
    <div className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Create a Commit Radar account</CardTitle>
          <CardDescription>The role cannot be changed later.</CardDescription>
        </CardHeader>
        <CardContent>
          <form id="register-form" onSubmit={handleSubmit}>
            <FieldGroup>
              <Field data-invalid={isEmailTaken}>
                <FieldLabel htmlFor="email">Email</FieldLabel>
                <Input
                  id="email"
                  name="email"
                  type="email"
                  autoComplete="email"
                  required
                  maxLength={254}
                  aria-invalid={isEmailTaken}
                  autoFocus
                />
                {isEmailTaken && <FieldError>{register.error?.message}</FieldError>}
              </Field>
              <Field>
                <FieldLabel htmlFor="displayName">Display name</FieldLabel>
                <Input id="displayName" name="displayName" autoComplete="name" required maxLength={100} />
              </Field>
              <Field>
                <FieldLabel htmlFor="password">Password</FieldLabel>
                <Input
                  id="password"
                  name="password"
                  type="password"
                  autoComplete="new-password"
                  required
                  minLength={8}
                  maxLength={72}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="role">Role</FieldLabel>
                <Select items={roleItems} value={role} onValueChange={(value) => value && setRole(value)}>
                  <SelectTrigger id="role" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {roleItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              {isCoordinator && (
                <Field>
                  <FieldLabel htmlFor="stream">Stream</FieldLabel>
                  <Select items={streamItems} value={stream} onValueChange={(value) => value && setStream(value)}>
                    <SelectTrigger id="stream" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {streamItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
              )}
              {hasOtherError && <FieldError>{register.error?.message}</FieldError>}
            </FieldGroup>
          </form>
        </CardContent>
        <CardFooter className="flex flex-col gap-2">
          <Button type="submit" form="register-form" className="w-full" disabled={register.isPending}>
            Register
          </Button>
          <Link to="/login" className={buttonVariants({ variant: 'link' })}>
            I already have an account
          </Link>
        </CardFooter>
      </Card>
    </div>
  )
}
