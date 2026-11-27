import { useMutation, useQuery } from "@tanstack/react-query";
import { Plus, Settings2, X } from "lucide-react";
import { useState, type ComponentType, type ReactElement } from "react";
import { toast } from "sonner";

import {
  api,
  canEditGlobalSettings,
  queryClient,
  settingQuery,
  type CommitSizeRange,
  type CommitSizeSettings,
  type JunkPatternsSetting,
  type RapidSeriesSettings,
  type SettingName,
  type SettingResponse,
  type SettingValues,
} from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSeparator,
  FieldSet,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { useCurrentUser } from "@/lib/hooks";
import { formatRange } from "@/lib/utils";

const minRapidSeriesCommits = 2;
const maxRapidSeriesCommits = 1000;
const maxRapidSeriesSeconds = 24 * 60 * 60;
const defaultLargeCommitLines = 1000;

interface SettingFormProps<Value> {
  formId: string;
  saved: Value;
  disabled: boolean;
  onSubmit: (value: Value) => void;
}

interface SettingsDialogProps<Name extends SettingName> {
  name: Name;
  title: string;
  description: string;
  triggerButton: ReactElement;
  triggerContent: React.ReactNode;
  form: ComponentType<SettingFormProps<SettingValues[Name]>>;
}

function SettingsDialog<Name extends SettingName>({
  name,
  title,
  description,
  triggerButton,
  triggerContent,
  form: SettingForm,
}: SettingsDialogProps<Name>) {
  const canEdit = canEditGlobalSettings(useCurrentUser());
  const [isOpen, setIsOpen] = useState(false);
  const setting = useQuery({ ...settingQuery(name), enabled: canEdit });

  function applySavedSetting(saved: SettingResponse<SettingValues[Name]>) {
    queryClient.setQueryData(settingQuery(name).queryKey, saved);
    void queryClient.invalidateQueries({ queryKey: ["analysis"] });
    void queryClient.invalidateQueries({ queryKey: ["students"] });
  }

  const save = useMutation({
    mutationFn: (value: SettingValues[Name]) => api.saveSetting(name, value),
    onSuccess: (saved) => {
      applySavedSetting(saved);
      setIsOpen(false);
      toast.success("Settings saved");
    },
  });

  const reset = useMutation({
    mutationFn: () => api.resetSetting(name),
    onSuccess: (saved) => {
      applySavedSetting(saved);
      toast.success("Default settings restored");
    },
    onError: (error) => toast.error(error.message),
  });

  function handleOpenChange(open: boolean) {
    setIsOpen(open);
    if (open) save.reset();
  }

  const formId = `${name}-form`;
  const isBusy = save.isPending || reset.isPending;
  const canReset = setting.isSuccess && !setting.data.isDefault && !isBusy;

  if (!canEdit) return null;

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogTrigger render={triggerButton}>{triggerContent}</DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {setting.isPending && (
          <p className="text-muted-foreground">Loading...</p>
        )}
        {setting.isError && (
          <Alert variant="destructive">
            <AlertDescription>{setting.error.message}</AlertDescription>
          </Alert>
        )}
        {setting.isSuccess && (
          // Remounting after a reset replaces the edited values with the saved ones
          <div
            key={setting.dataUpdatedAt}
            className="max-h-[60vh] overflow-y-auto p-1"
          >
            <SettingForm
              formId={formId}
              saved={setting.data.value}
              disabled={isBusy}
              onSubmit={(value) => save.mutate(value)}
            />
            <FieldError className="pt-4">{save.error?.message}</FieldError>
          </div>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            disabled={!canReset}
            onClick={() => reset.mutate()}
          >
            Reset to defaults
          </Button>
          <Button
            type="submit"
            form={formId}
            disabled={isBusy || !setting.isSuccess}
          >
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

const chartSettingsButton = (
  <Button variant="ghost" size="icon" title="Settings" />
);

const chartSettingsContent = (
  <>
    <Settings2 />
    <span className="sr-only">Settings</span>
  </>
);

export function JunkPatternsDialog() {
  return (
    <SettingsDialog
      name="junk-patterns"
      title="Junk patterns"
      description="Files matching these patterns are shown as junk and excluded from code statistics. Use glob patterns such as **/node_modules/**, or <binary> for every binary file."
      triggerButton={<Button variant="link" />}
      triggerContent="Edit patterns"
      form={JunkPatternsForm}
    />
  );
}

export function CommitSizesDialog() {
  return (
    <SettingsDialog
      name="commit-sizes"
      title="Commit size settings"
      description="Ranges of changed lines for the chart and the limit for large commits."
      triggerButton={chartSettingsButton}
      triggerContent={chartSettingsContent}
      form={CommitSizesForm}
    />
  );
}

export function RapidSeriesDialog() {
  return (
    <SettingsDialog
      name="rapid-series"
      title="Rapid series settings"
      description="Commits form a rapid series when they match any enabled rule."
      triggerButton={chartSettingsButton}
      triggerContent={chartSettingsContent}
      form={RapidSeriesForm}
    />
  );
}

let nextRowId = 0;

interface PatternRow {
  id: number;
  value: string;
}

function JunkPatternsForm({
  formId,
  saved,
  disabled,
  onSubmit,
}: SettingFormProps<JunkPatternsSetting>) {
  const [rows, setRows] = useState<PatternRow[]>(() =>
    saved.patterns.map((value) => ({ id: nextRowId++, value })),
  );

  function updateRow(id: number, value: string) {
    setRows((current) =>
      current.map((row) => (row.id === id ? { ...row, value } : row)),
    );
  }

  function removeRow(id: number) {
    setRows((current) => current.filter((row) => row.id !== id));
  }

  function addRow() {
    setRows((current) => [...current, { id: nextRowId++, value: "" }]);
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const patterns = rows
      .map((row) => row.value.trim())
      .filter((pattern) => pattern !== "");
    onSubmit({ patterns });
  }

  return (
    <form id={formId} onSubmit={handleSubmit}>
      <FieldGroup>
        {rows.map((row, index) => (
          <Field key={row.id} orientation="horizontal">
            <Input
              value={row.value}
              disabled={disabled}
              aria-label={`Pattern ${index + 1}`}
              onChange={(event) => updateRow(row.id, event.target.value)}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              disabled={disabled}
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
          <Button
            type="button"
            variant="outline"
            disabled={disabled}
            onClick={addRow}
          >
            <Plus />
            Add pattern
          </Button>
        </div>
      </FieldGroup>
    </form>
  );
}

// Mirrors the server validation, so most mistakes are shown before saving
function commitSizeRangesError(ranges: CommitSizeRange[]) {
  if (ranges.length === 0) return "Add at least one range.";

  const openRanges = ranges.filter((range) => range.to === null);
  if (openRanges.length > 1) return "Only one range can have no upper limit.";

  const reversedRange = ranges.find(
    (range) => range.to !== null && range.to < range.from,
  );
  if (reversedRange)
    return `Range ${formatRange(reversedRange)} ends before it starts.`;

  const sortedRanges = [...ranges].sort((a, b) => a.from - b.from);
  for (let i = 1; i < sortedRanges.length; i++) {
    const previous = sortedRanges[i - 1];
    const current = sortedRanges[i];
    const isOverlapping = previous.to === null || previous.to >= current.from;
    if (isOverlapping)
      return `Ranges ${formatRange(previous)} and ${formatRange(current)} overlap.`;
  }

  return null;
}

interface RangeRow {
  id: number;
  from: string;
  to: string;
}

function rowStart(row: RangeRow) {
  return row.from === "" ? Infinity : Number(row.from);
}

function sortedByStart(rows: RangeRow[]) {
  return [...rows].sort((a, b) => rowStart(a) - rowStart(b));
}

function toRange(row: RangeRow): CommitSizeRange {
  return { from: Number(row.from), to: row.to === "" ? null : Number(row.to) };
}

type LargeCommitRule = "largest-range" | "line-limit";

function CommitSizesForm({
  formId,
  saved,
  disabled,
  onSubmit,
}: SettingFormProps<CommitSizeSettings>) {
  const [rows, setRows] = useState<RangeRow[]>(() =>
    saved.ranges.map((range) => ({
      id: nextRowId++,
      from: String(range.from),
      to: range.to === null ? "" : String(range.to),
    })),
  );
  const [largeCommitRule, setLargeCommitRule] = useState<LargeCommitRule>(
    saved.largeCommitLines === null ? "largest-range" : "line-limit",
  );
  const [largeCommitLines, setLargeCommitLines] = useState(
    String(saved.largeCommitLines ?? defaultLargeCommitLines),
  );
  const [validationError, setValidationError] = useState<string | null>(null);

  const isLineLimitRule = largeCommitRule === "line-limit";
  const largestRangeRow = sortedByStart(rows).findLast(
    (row) => row.from !== "",
  );

  function updateRow(id: number, change: Partial<RangeRow>) {
    setRows((current) =>
      current.map((row) => (row.id === id ? { ...row, ...change } : row)),
    );
  }

  function removeRow(id: number) {
    setRows((current) => current.filter((row) => row.id !== id));
  }

  function addRow() {
    setRows((current) => [...current, { id: nextRowId++, from: "", to: "" }]);
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const ranges = sortedByStart(rows).map(toRange);
    const error = commitSizeRangesError(ranges);
    setValidationError(error);
    if (error) return;

    onSubmit({
      ranges,
      largeCommitLines: isLineLimitRule ? Number(largeCommitLines) : null,
    });
  }

  return (
    <form id={formId} onSubmit={handleSubmit}>
      <FieldGroup>
        <FieldSet>
          <FieldLegend>Ranges</FieldLegend>
          <FieldDescription>
            Changed lines from and to, both included. Leave the end empty for
            the range without an upper limit.
          </FieldDescription>
          {/* Sorting on blur keeps the row in place while its start is typed */}
          <FieldGroup onBlur={() => setRows(sortedByStart)}>
            {rows.map((row, index) => (
              <Field key={row.id} orientation="horizontal">
                <Input
                  type="number"
                  min={0}
                  step={1}
                  required
                  value={row.from}
                  disabled={disabled}
                  aria-label={`Range ${index + 1} start`}
                  onChange={(event) =>
                    updateRow(row.id, { from: event.target.value })
                  }
                />
                <span>-</span>
                <Input
                  type="number"
                  min={0}
                  step={1}
                  value={row.to}
                  placeholder="No limit"
                  disabled={disabled}
                  aria-label={`Range ${index + 1} end`}
                  onChange={(event) =>
                    updateRow(row.id, { to: event.target.value })
                  }
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  disabled={disabled}
                  aria-label={`Remove range ${index + 1}`}
                  onClick={() => removeRow(row.id)}
                >
                  <X />
                </Button>
              </Field>
            ))}
          </FieldGroup>
          <div>
            <Button
              type="button"
              variant="outline"
              disabled={disabled}
              onClick={addRow}
            >
              <Plus />
              Add range
            </Button>
          </div>
          <FieldError>{validationError}</FieldError>
        </FieldSet>
        <FieldSeparator />
        <FieldSet>
          <FieldLegend>Large commits</FieldLegend>
          <RadioGroup
            value={largeCommitRule}
            disabled={disabled}
            onValueChange={(value) =>
              setLargeCommitRule(
                value === "line-limit" ? "line-limit" : "largest-range",
              )
            }
          >
            <Field orientation="horizontal">
              <RadioGroupItem value="largest-range" id="large-largest-range" />
              <FieldLabel htmlFor="large-largest-range">
                Commits in the largest range
                {largestRangeRow &&
                  ` (${formatRange(toRange(largestRangeRow))})`}
              </FieldLabel>
            </Field>
            <Field orientation="horizontal">
              <RadioGroupItem value="line-limit" id="large-line-limit" />
              <FieldLabel htmlFor="large-line-limit">
                Commits with more changed lines than
              </FieldLabel>
              <Input
                type="number"
                min={0}
                step={1}
                required={isLineLimitRule}
                value={largeCommitLines}
                disabled={disabled || !isLineLimitRule}
                aria-label="Large commit line limit"
                className="w-28"
                onChange={(event) => setLargeCommitLines(event.target.value)}
              />
            </Field>
          </RadioGroup>
        </FieldSet>
      </FieldGroup>
    </form>
  );
}

interface RuleDraft {
  enabled: boolean;
  minCommits: string;
  seconds: string;
}

function RapidSeriesRuleFields({
  id,
  title,
  secondsLabel,
  rule,
  disabled,
  onChange,
}: {
  id: string;
  title: string;
  secondsLabel: string;
  rule: RuleDraft;
  disabled: boolean;
  onChange: (rule: RuleDraft) => void;
}) {
  const areValuesDisabled = disabled || !rule.enabled;

  return (
    <FieldSet>
      <Field orientation="horizontal">
        <Checkbox
          id={`${id}-enabled`}
          checked={rule.enabled}
          disabled={disabled}
          onCheckedChange={(enabled) => onChange({ ...rule, enabled })}
        />
        <FieldLabel htmlFor={`${id}-enabled`}>{title}</FieldLabel>
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor={`${id}-commits`}>Commits, at least</FieldLabel>
          <Input
            id={`${id}-commits`}
            type="number"
            min={minRapidSeriesCommits}
            max={maxRapidSeriesCommits}
            step={1}
            required
            value={rule.minCommits}
            disabled={areValuesDisabled}
            onChange={(event) =>
              onChange({ ...rule, minCommits: event.target.value })
            }
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-seconds`}>{secondsLabel}</FieldLabel>
          <Input
            id={`${id}-seconds`}
            type="number"
            min={1}
            max={maxRapidSeriesSeconds}
            step={1}
            required
            value={rule.seconds}
            disabled={areValuesDisabled}
            onChange={(event) =>
              onChange({ ...rule, seconds: event.target.value })
            }
          />
        </Field>
      </div>
    </FieldSet>
  );
}

function RapidSeriesForm({
  formId,
  saved,
  disabled,
  onSubmit,
}: SettingFormProps<RapidSeriesSettings>) {
  const [consecutiveRule, setConsecutiveRule] = useState<RuleDraft>({
    enabled: saved.consecutive.enabled,
    minCommits: String(saved.consecutive.minCommits),
    seconds: String(saved.consecutive.maxGapSeconds),
  });
  const [windowRule, setWindowRule] = useState<RuleDraft>({
    enabled: saved.window.enabled,
    minCommits: String(saved.window.minCommits),
    seconds: String(saved.window.windowSeconds),
  });
  const [validationError, setValidationError] = useState<string | null>(null);

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const isAnyRuleEnabled = consecutiveRule.enabled || windowRule.enabled;
    if (!isAnyRuleEnabled) {
      setValidationError("Enable at least one rule.");
      return;
    }

    setValidationError(null);
    onSubmit({
      consecutive: {
        enabled: consecutiveRule.enabled,
        minCommits: Number(consecutiveRule.minCommits),
        maxGapSeconds: Number(consecutiveRule.seconds),
      },
      window: {
        enabled: windowRule.enabled,
        minCommits: Number(windowRule.minCommits),
        windowSeconds: Number(windowRule.seconds),
      },
    });
  }

  return (
    <form id={formId} onSubmit={handleSubmit}>
      <FieldGroup>
        <RapidSeriesRuleFields
          id="consecutive"
          title="Each commit soon after the previous one"
          secondsLabel="Max gap in seconds"
          rule={consecutiveRule}
          disabled={disabled}
          onChange={setConsecutiveRule}
        />
        <FieldSeparator />
        <RapidSeriesRuleFields
          id="window"
          title="Many commits in a short time"
          secondsLabel="Window in seconds"
          rule={windowRule}
          disabled={disabled}
          onChange={setWindowRule}
        />
        <FieldError>{validationError}</FieldError>
      </FieldGroup>
    </form>
  );
}
