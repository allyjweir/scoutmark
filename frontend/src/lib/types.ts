// ─── Domain Types ──────────────────────────────────────────────────

export interface User {
  id: string;
  username: string;
  display_name: string;
  is_admin: boolean;
}

export interface Session {
  id: string;
  event_id: string;
  event_name: string;
  template_id: string;
  name: string;
  starts_at: string;
  ends_at: string;
  status: 'UPCOMING' | 'ACTIVE' | 'CLOSED';
  created_at: string;
  criteria?: Criterion[];
  user_finalised: boolean;
  previous_session_id: string | null;
  award_best_patrol: boolean;
  award_most_improved: boolean;
}

export interface Award {
  award_type: 'best_patrol' | 'most_improved';
  patrol_id: string;
}

export interface PreviousPatrolTotal {
  patrol_id: string;
  patrol_name: string;
  total: number;
}

export interface Criterion {
  id: string;
  title: string;
  description: string;
  min_value: number;
  max_value: number;
  sort_order: number;
}

export interface Patrol {
  patrol_id: string;
  name: string;
  sort_order: number;
}

export interface Draft {
  id: string;
  patrol_id: string;
  scores: DraftScore[];
  updated_at: string;
}

export interface DraftScore {
  criterion_id: string;
  value: number;
}

export interface Submission {
  id: string;
  patrol_id: string;
  patrol_name: string;
  locked: boolean;
  submitted_at: string;
  scores?: SubmissionScore[];
}

export interface SubmissionScore {
  criterion_id: string;
  value: number;
}

// ─── WebSocket Types ───────────────────────────────────────────────

export interface WSClientMessage {
  request_id: string;
  type: 'save_draft' | 'subscribe_session';
  payload: unknown;
}

export interface WSServerMessage {
  request_id?: string;
  type: 'draft_saved' | 'patrol_submitted' | 'error' | 'subscribed' | 'progress_updated';
  payload: unknown;
}

export interface WSSaveDraftPayload {
  session_id: string;
  patrol_id: string;
  scores: Record<string, number>;
}

export interface WSDraftSavedPayload {
  session_id: string;
  patrol_id: string;
  saved_at: string;
}

export interface WSPatrolSubmittedPayload {
  session_id: string;
  patrol_id: string;
  user_display_name: string;
  submitted_at: string;
}

export interface WSErrorPayload {
  code: string;
  message: string;
}

export interface WSPatrolProgressPayload {
  patrol_id: string;
  patrol_name: string;
  status: 'not_started' | 'drafting' | 'submitted';
}

export interface WSUserProgressPayload {
  user_id: string;
  display_name: string;
  patrols: WSPatrolProgressPayload[];
}

export interface WSProgressUpdatedPayload {
  session_id: string;
  users: WSUserProgressPayload[];
}
