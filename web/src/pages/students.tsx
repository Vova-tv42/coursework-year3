import { useMemo, useState, type SubmitEvent } from 'react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import {
  createColumnHelper,
  createSortedRowModel,
  rowSortingFeature,
  sortFns,
  tableFeatures,
  useTable,
} from '@tanstack/react-table'
import { ArrowDown, ArrowUp, ArrowUpDown, MoreHorizontal, Pencil, Plus, Trash2 } from 'lucide-react'
import { Link } from 'react-router'
import { toast } from 'sonner'

import {
  api,
  queryClient,
  statusLabels,
  streamLabels,
  studentsQuery,
  type AnalysisStatus,
  type Stream,
  type StudentInput,
  type StudentOverview,
} from '@/api/client'
import { useCurrentUser } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const streamItems = Object.entries(streamLabels).map(([value, label]) => ({ value: value as Stream, label }))
const streamFilterItems = [{ value: null, label: 'All streams' }, ...streamItems]

const statusBadgeVariants: Record<AnalysisStatus, 'outline' | 'secondary' | 'default' | 'destructive'> = {
  never: 'outline',
  queued: 'secondary',
  running: 'secondary',
  done: 'default',
  failed: 'destructive',
}

const lastCommitFormat = new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeZone: 'Europe/Kyiv' })

const features = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns,
})
const columnHelper = createColumnHelper<typeof features, StudentOverview>()

function createColumns(onEdit: (student: StudentOverview) => void, onDelete: (student: StudentOverview) => void) {
  return columnHelper.columns([
    columnHelper.accessor('fullName', {
      header: 'Full name',
      cell: (info) => (
        <Link to={`/students/${info.row.original.id}`} className={cn(buttonVariants({ variant: 'link' }), 'px-0')}>
          {info.getValue()}
        </Link>
      ),
    }),
    columnHelper.accessor('stream', {
      header: 'Stream',
      cell: (info) => streamLabels[info.getValue()],
    }),
    columnHelper.accessor('supervisorName', { header: 'Supervisor' }),
    columnHelper.accessor((student) => student.repository.lastCommitAt ?? undefined, {
      id: 'lastCommitAt',
      header: 'Last commit',
      sortUndefined: 'last',
      cell: (info) => {
        const lastCommitAt = info.getValue()
        return lastCommitAt ? lastCommitFormat.format(new Date(lastCommitAt)) : '-'
      },
    }),
    columnHelper.accessor((student) => student.repository.status, {
      id: 'status',
      header: 'Analysis',
      cell: (info) => <Badge variant={statusBadgeVariants[info.getValue()]}>{statusLabels[info.getValue()]}</Badge>,
    }),
    columnHelper.display({
      id: 'actions',
      cell: ({ row }) =>
        row.original.canEdit && (
          <DropdownMenu>
            <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" />}>
              <MoreHorizontal />
              <span className="sr-only">Student actions</span>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => onEdit(row.original)}>
                <Pencil />
                Edit
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onClick={() => onDelete(row.original)}>
                <Trash2 />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ),
    }),
  ])
}

async function refreshStudents(studentId?: number) {
  await queryClient.invalidateQueries({ queryKey: ['students'] })
  if (studentId !== undefined) {
    queryClient.removeQueries({ queryKey: ['student', String(studentId)] })
  }
}

function StudentFormDialog({ student, onClose }: { student: StudentOverview | null; onClose: () => void }) {
  const isEditing = student !== null
  const [stream, setStream] = useState<Stream>(student?.stream ?? 'bachelor')

  const save = useMutation({
    mutationFn: (input: StudentInput) => (student ? api.updateStudent(student.id, input) : api.createStudent(input)),
    onSuccess: async (saved) => {
      await refreshStudents(saved.id)
      toast.success(isEditing ? 'Student updated' : 'Student added')
      onClose()
    },
  })

  function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    save.mutate({
      fullName: String(form.get('fullName')),
      stream,
      cloneUrl: String(form.get('cloneUrl')).trim(),
    })
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{isEditing ? 'Edit student' : 'Add student'}</DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Changing the clone URL removes the collected analysis of the old repository.'
              : 'The repository is analyzed after you start the analysis on the student page.'}
          </DialogDescription>
        </DialogHeader>
        <form id="student-form" onSubmit={handleSubmit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="fullName">Full name</FieldLabel>
              <Input id="fullName" name="fullName" defaultValue={student?.fullName} required maxLength={200} autoFocus />
            </Field>
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
            <Field>
              <FieldLabel htmlFor="cloneUrl">Clone URL</FieldLabel>
              <Input
                id="cloneUrl"
                name="cloneUrl"
                defaultValue={student?.repository.cloneUrl}
                placeholder="https://github.com/user/thesis.git"
                required
              />
            </Field>
            <FieldError>{save.error?.message}</FieldError>
          </FieldGroup>
        </form>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="student-form" disabled={save.isPending}>
            {isEditing ? 'Save' : 'Add'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DeleteStudentDialog({ student, onClose }: { student: StudentOverview; onClose: () => void }) {
  const remove = useMutation({
    mutationFn: () => api.deleteStudent(student.id),
    onSuccess: async () => {
      await refreshStudents(student.id)
      toast.success('Student deleted')
      onClose()
    },
    onError: (error) => toast.error(error.message),
  })

  return (
    <AlertDialog open onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {student.fullName}?</AlertDialogTitle>
          <AlertDialogDescription>
            The student, the repository and all collected analysis are removed. This cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <Button variant="destructive" disabled={remove.isPending} onClick={() => remove.mutate()}>
            Delete
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function SortIcon({ direction }: { direction: false | 'asc' | 'desc' }) {
  if (direction === 'asc') return <ArrowUp />
  if (direction === 'desc') return <ArrowDown />
  return <ArrowUpDown />
}

export function StudentsPage() {
  const user = useCurrentUser()
  const canAddStudents = user.role === 'supervisor'
  const [streamFilter, setStreamFilter] = useState<Stream | null>(null)
  const [formStudent, setFormStudent] = useState<StudentOverview | null>(null)
  const [isFormOpen, setIsFormOpen] = useState(false)
  const [deletingStudent, setDeletingStudent] = useState<StudentOverview | null>(null)

  const students = useQuery({ ...studentsQuery(streamFilter), placeholderData: keepPreviousData })

  const columns = useMemo(
    () =>
      createColumns(
        (student) => {
          setFormStudent(student)
          setIsFormOpen(true)
        },
        setDeletingStudent,
      ),
    [],
  )
  const table = useTable({ features, columns, data: students.data ?? [] })

  function openAddDialog() {
    setFormStudent(null)
    setIsFormOpen(true)
  }

  const emptyMessage = streamFilter
    ? 'No students in this stream.'
    : canAddStudents
      ? 'No students yet. Add the first one to start tracking a thesis repository.'
      : 'No students are visible to you yet.'

  return (
    <Card>
      <CardHeader>
        <CardTitle>Students</CardTitle>
        <CardAction className="flex gap-2">
          <Select items={streamFilterItems} value={streamFilter} onValueChange={(value) => setStreamFilter(value)}>
            <SelectTrigger aria-label="Stream filter">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {streamFilterItems.map((item) => (
                <SelectItem key={item.label} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {canAddStudents && (
            <Button onClick={openAddDialog}>
              <Plus />
              Add student
            </Button>
          )}
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {students.isError && (
          <Alert variant="destructive">
            <AlertDescription>{students.error.message}</AlertDescription>
          </Alert>
        )}
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead key={header.id}>
                    {header.column.getCanSort() ? (
                      <Button variant="ghost" size="sm" onClick={header.column.getToggleSortingHandler()}>
                        <table.FlexRender header={header} />
                        <SortIcon direction={header.column.getIsSorted()} />
                      </Button>
                    ) : (
                      <table.FlexRender header={header} />
                    )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.map((row) => (
              <TableRow key={row.id}>
                {row.getAllCells().map((cell) => (
                  <TableCell key={cell.id}>
                    <table.FlexRender cell={cell} />
                  </TableCell>
                ))}
              </TableRow>
            ))}
            {students.isSuccess && students.data.length === 0 && (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                  {emptyMessage}
                </TableCell>
              </TableRow>
            )}
            {students.isPending && (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                  Loading...
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>

      {isFormOpen && <StudentFormDialog student={formStudent} onClose={() => setIsFormOpen(false)} />}
      {deletingStudent && <DeleteStudentDialog student={deletingStudent} onClose={() => setDeletingStudent(null)} />}
    </Card>
  )
}
