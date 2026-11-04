import { useQuery } from "@tanstack/react-query";

import { checklistQuery, type ChecklistItemKey } from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const itemLabels: Record<ChecklistItemKey, string> = {
  code: "Project code",
  explanatory_note: "Explanatory note",
  presentation: "Presentation",
  readme_deployment: "README",
};

// Code and README matches are obvious, but a reviewer should see which document was accepted
const itemsWithShownFiles: ChecklistItemKey[] = [
  "explanatory_note",
  "presentation",
];

export function AdmissionBadge({ admitted }: { admitted: boolean }) {
  return admitted ? (
    <Badge>Admitted</Badge>
  ) : (
    <Badge variant="destructive">Not admitted</Badge>
  );
}

export function Checklist({ repositoryId }: { repositoryId: number }) {
  const checklist = useQuery(checklistQuery(repositoryId));

  if (checklist.isPending)
    return <p className="text-muted-foreground">Loading...</p>;
  if (checklist.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{checklist.error.message}</AlertDescription>
      </Alert>
    );
  }

  if (!checklist.data.evaluated) {
    return (
      <p className="text-muted-foreground">
        The checklist is evaluated after the first successful analysis.
      </p>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Defense checklist</CardTitle>
        <CardDescription>
          Checked on the latest state of the default branch.
        </CardDescription>
        <CardAction>
          <AdmissionBadge admitted={checklist.data.admitted} />
        </CardAction>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Item</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {checklist.data.items.map((item) => (
              <TableRow key={item.key}>
                <TableCell className="whitespace-normal">
                  <div>{itemLabels[item.key]}</div>
                  {itemsWithShownFiles.includes(item.key) &&
                    item.files.map((file) => (
                      <div
                        key={file}
                        className="text-muted-foreground break-all"
                      >
                        {file}
                      </div>
                    ))}
                </TableCell>
                <TableCell>
                  {item.found ? (
                    <Badge variant="secondary">Found</Badge>
                  ) : (
                    <Badge variant="destructive">Missing</Badge>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
