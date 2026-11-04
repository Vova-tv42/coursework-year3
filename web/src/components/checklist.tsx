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
          Checked on the latest state of the default branch. Up to 20 files are
          shown per item.
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
              <TableHead>Matched files</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {checklist.data.items.map((item) => (
              <TableRow key={item.key}>
                <TableCell>{itemLabels[item.key]}</TableCell>
                <TableCell>
                  {item.found ? (
                    <Badge variant="secondary">Found</Badge>
                  ) : (
                    <Badge variant="destructive">Missing</Badge>
                  )}
                </TableCell>
                <TableCell className="whitespace-normal">
                  {item.files.length > 0 ? (
                    <ul>
                      {item.files.map((file) => (
                        <li key={file} className="break-all">
                          {file}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    "-"
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
