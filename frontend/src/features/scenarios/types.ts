export type AutomationCandidate = {
  id: string;
  title: string;
  type?: string;
  rationale: string;
  affectedSteps?: string[];
  impact: string;
  confidence: number;
  score: number;
  status: string;
  breakdown?: {
    level?: string;
    frequency?: number;
    averageDurationMs?: number;
    durationImpactMs?: number;
    repeatability?: number;
    manualCheckImpact?: number;
    errorReduction?: number;
    dataQualityConfidence?: number;
    sampleSize?: number;
    factors?: Record<string, number>;
    weights?: Record<string, number>;
  };
};

export type ScenarioGroup = {
  id: string;
  code?: string;
  name: string;
  issueType: string;
  signature: string;
  frequency: number;
  averageDurationMs: number;
  medianDurationMs: number;
  p95DurationMs: number;
  manualCheckCount: number;
  repeatedActionCount: number;
  confidenceAverage: number;
  ambiguousCount: number;
  automationScore: number;
  actionSequence: string[];
  variants?: { sequence: string[]; frequency: number }[];
  status: string;
  automationCandidates?: AutomationCandidate[] | null;
};
