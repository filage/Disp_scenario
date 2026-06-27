export type ActionEvent = {
  id: string;
  recordingId: string;
  analysisRunId?: string;
  timestampMs: number;
  canonicalAction: string;
  eventType: string;
  screen: string;
  entityType?: string;
  entityId?: string;
  orderId?: string;
  issueType?: string;
  target: string;
  payload?: Record<string, unknown>;
  confidence: number;
  sourceRawEventIds?: string[];
  qualityFlags?: string[];
  qaStatus?: string;
  qaComment?: string;
  version: number;
};

export type RawEvent = {
  id: string;
  timestampMs: number;
  screen: string;
  visibleText?: string;
  target?: string;
  eventTypeGuess: string;
  colorCues?: string[];
  stateChange?: string;
  confidence: number;
  payload?: Record<string, unknown>;
};

export type QualityIssue = {
  id: string;
  type: string;
  severity: string;
  message: string;
  timestampMs: number;
  resolved: boolean;
  actionEventId?: string;
};

export type ScenarioInstance = {
  id: string;
  recordingId: string;
  analysisRunId?: string;
  templateId?: string;
  knownScenarioCode?: string;
  orderId?: string;
  entityType?: string;
  entityId?: string;
  issueType: string;
  startedAtMs: number;
  endedAtMs: number;
  durationMs: number;
  eventIds?: string[];
  outcome: string;
  status: string;
  confidence: number;
  boundaryRuleVersion?: string;
  qualityFlags?: string[];
};

export type AnalysisBundle = {
  events: ActionEvent[];
  rawEvents: RawEvent[];
  groundTruth?: Record<string, unknown>[];
  dataQualityIssues: QualityIssue[];
  scenarios?: {
    instances?: ScenarioInstance[];
  };
};

export const ACTION_OPTIONS = [
  "NAVIGATE",
  "FILTER_ISSUES",
  "INSPECT_ISSUE",
  "OPEN_ORDER",
  "TAKE_ACTION",
  "CHECK",
  "OPEN_DRIVER_ASSIGNMENT",
  "SELECT_DRIVER",
  "SEND_TO_SELECTED_DRIVER",
  "ASSIGN_DRIVER",
  "MARK_PICKUP_COMPLETED",
  "OPEN_FIELD_EDITOR",
  "CHANGE_FIELD_VALUE",
  "EDIT_FIELD",
  "SAVE",
  "RESOLUTION_ATTEMPT",
  "RESOLVE_ISSUE",
];
