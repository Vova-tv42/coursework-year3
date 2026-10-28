import { useMutation } from '@tanstack/react-query'
import { LogOut, Moon, Sun } from 'lucide-react'
import { NavLink, Outlet, useNavigate, useRouteLoaderData } from 'react-router'
import { toast } from 'sonner'

import { api, queryClient, roleLabels, streamLabels, type User } from '@/api/client'
import { useTheme } from '@/components/theme-provider'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'

export const signedInRouteId = 'signed-in'

export function useCurrentUser() {
  const user = useRouteLoaderData(signedInRouteId) as User | undefined
  if (!user) throw new Error('useCurrentUser must be used inside the signed-in route')
  return user
}

function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const nextTheme = theme === 'dark' ? 'light' : 'dark'

  return (
    <Button variant="outline" size="icon" title={`Switch to ${nextTheme} theme`} onClick={() => setTheme(nextTheme)}>
      {theme === 'dark' ? <Sun /> : <Moon />}
      <span className="sr-only">Switch to {nextTheme} theme</span>
    </Button>
  )
}

export function Layout() {
  const user = useCurrentUser()
  const navigate = useNavigate()
  const roleDescription = user.stream
    ? `${roleLabels[user.role]}, ${streamLabels[user.stream].toLowerCase()}`
    : roleLabels[user.role]

  const signOut = useMutation({
    mutationFn: api.logout,
    onSuccess: async () => {
      queryClient.clear()
      await navigate('/login')
    },
    onError: (error) => toast.error(error.message),
  })

  return (
    <>
      <header className="mx-auto flex max-w-6xl items-center gap-2 px-4 py-3">
        <span className="mr-4 font-semibold">Commit Radar</span>
        <nav>
          <NavLink to="/" end className={buttonVariants({ variant: 'ghost' })}>
            Students
          </NavLink>
        </nav>
        <div className="ml-auto flex items-center gap-2">
          <span className="hidden sm:inline">{user.displayName}</span>
          <Badge variant="secondary" className="hidden sm:inline-flex">
            {roleDescription}
          </Badge>
          <ThemeToggle />
          <Button
            variant="ghost"
            size="icon"
            title="Sign out"
            disabled={signOut.isPending}
            onClick={() => signOut.mutate()}
          >
            <LogOut />
            <span className="sr-only">Sign out</span>
          </Button>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6">
        <Outlet />
      </main>
    </>
  )
}
