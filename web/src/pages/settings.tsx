import { useMutation, useQuery } from "@tanstack/react-query";
import { Plus, X } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import {
  api,
  junkPatternsQuery,
  queryClient,
  type JunkPatterns,
} from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Field, FieldError, FieldGroup } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

interface PatternRow {
  id: number;
  value: string;
}

let nextRowId = 0;

function toRows(patterns: string[]): PatternRow[] {
  return patterns.map((value) => ({ id: nextRowId++, value }));
}

function applySavedPatterns(saved: JunkPatterns) {
  queryClient.setQueryData(junkPatternsQuery.queryKey, saved);
  void queryClient.invalidateQueries({ queryKey: ["analysis"] });
}

export function SettingsPage() {
  const junkPatterns = useQuery(junkPatternsQuery);

  if (junkPatterns.isPending)
    return <p className="text-muted-foreground">Loading...</p>;
  if (junkPatterns.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{junkPatterns.error.message}</AlertDescription>
      </Alert>
    );
  }

  // Remounting on every update resets the edited rows to the saved list
  return (
    <JunkPatternsForm
      key={junkPatterns.dataUpdatedAt}
      saved={junkPatterns.data}
    />
  );
}

function JunkPatternsForm({ saved }: { saved: JunkPatterns }) {
  const [rows, setRows] = useState(() => toRows(saved.patterns));

  const save = useMutation({
    mutationFn: api.setJunkPatterns,
    onSuccess: (result) => {
      applySavedPatterns(result);
      toast.success("Junk patterns saved");
    },
  });

  const reset = useMutation({
    mutationFn: api.resetJunkPatterns,
    onSuccess: (result) => {
      applySavedPatterns(result);
      toast.success("Default junk patterns restored");
    },
    onError: (error) => toast.error(error.message),
  });

  function updateRow(id: number, value: string) {
    setRows((current) =>
      current.map((row) => (row.id === id ? { ...row, value } : row)),
    );
  }

  function removeRow(id: number) {
    setRows((current) => current.filter((row) => row.id !== id));
  }

  function addRow() {
    setRows((current) => [...current, ...toRows([""])]);
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const patterns = rows
      .map((row) => row.value.trim())
      .filter((pattern) => pattern !== "");
    save.mutate(patterns);
  }

  const isBusy = save.isPending || reset.isPending;

  return (
    <Card className="mx-auto max-w-2xl">
      <CardHeader>
        <CardTitle>Junk patterns</CardTitle>
        <CardDescription>
          Files matching these patterns are shown as junk and excluded from code
          statistics. Use glob patterns such as **/node_modules/**, or
          &lt;binary&gt; for every binary file. The list applies only to you.
        </CardDescription>
        {saved.isDefault && (
          <CardAction>
            <Badge variant="secondary">Defaults</Badge>
          </CardAction>
        )}
      </CardHeader>
      <CardContent>
        <form id="junk-patterns-form" onSubmit={handleSubmit}>
          <FieldGroup>
            {rows.map((row, index) => (
              <Field key={row.id} orientation="horizontal">
                <Input
                  value={row.value}
                  aria-label={`Pattern ${index + 1}`}
                  onChange={(event) => updateRow(row.id, event.target.value)}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`Remove pattern ${index + 1}`}
                  onClick={() => removeRow(row.id)}
                >
                  <X />
                </Button>
              </Field>
            ))}
            {rows.length === 0 && (
              <p className="text-muted-foreground">
                No patterns: every file is counted in the statistics.
              </p>
            )}
            <div>
              <Button type="button" variant="outline" onClick={addRow}>
                <Plus />
                Add pattern
              </Button>
            </div>
            <FieldError>{save.error?.message}</FieldError>
          </FieldGroup>
        </form>
      </CardContent>
      <CardFooter className="gap-2">
        <Button type="submit" form="junk-patterns-form" disabled={isBusy}>
          Save
        </Button>
        <Button
          variant="outline"
          disabled={isBusy || saved.isDefault}
          onClick={() => reset.mutate()}
        >
          Reset to defaults
        </Button>
      </CardFooter>
    </Card>
  );
}
