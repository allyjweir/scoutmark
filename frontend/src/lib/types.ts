// ─── Domain Types ──────────────────────────────────────────────────

export interface User {
  id: string;
  username: string;
  display_name: string;
  is_admin: boolean;
  subcamp_id?: string;
  password_change_required?: boolean;
}

export interface Session {
  id: string;
  event_id: string;
  event_name: string;
  template_id: string;
  name: string;
  starts_at: string;
  ends_at: string;
  status: 'UPCOMING' | 'ACTIVE' | 'LOCKED' | 'CLOSED';
  locked_at?: string;
  locked_by?: string;
  locked_by_name?: string;
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
  subcamp_id?: string;
  subcamp?: string;
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
  submitted_by?: string;
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
  type: 'save_draft' | 'subscribe_session' | 'unsubscribe_session' | 'presence';
  payload: unknown;
}

// Per-user comment on a criterion
export interface DraftComment {
  criterion_id: string;
  user_id: string;
  display_name: string;
  comment: string;
  updated_at: string;
}

// WebSocket: comment updated broadcast
export interface WSCommentUpdatedPayload {
  session_id: string;
  patrol_id: string;
  criterion_id: string;
  user_id: string;
  display_name: string;
  comment: string;
}

export interface WSServerMessage {
  request_id?: string;
  type: 'draft_saved' | 'draft_updated' | 'patrol_submitted' | 'error' | 'subscribed' | 'unsubscribed' | 'progress_updated' | 'presence_updated' | 'presence_state' | 'comment_updated' | 'session_finalised' | 'session_locked' | 'session_unlocked';
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

/** Broadcast to OTHER users when someone saves a draft score (live multiplayer). */
export interface WSDraftUpdatedPayload {
  session_id: string;
  patrol_id: string;
  user_id: string;
  user_name: string;
  scores: Record<string, number>;
  saved_at: string;
}

export interface WSPatrolSubmittedPayload {
  session_id: string;
  patrol_id: string;
  user_display_name: string;
  submitted_at: string;
}

export interface WSSessionFinalisedPayload {
  session_id: string;
  user_id: string;
  user_display_name: string;
  subcamp_id: string;
  finalised_at: string;
  ends_at: string;
}

export interface WSSessionLockedPayload {
  session_id: string;
  user_id: string;
  user_display_name: string;
  locked_at: string;
  ends_at: string;
}

export interface WSSessionUnlockedPayload {
  session_id: string;
}

export interface WSPresenceUpdatedPayload {
  session_id: string;
  patrol_id: string;
  user_id: string;
  user_name: string;
  commenting_on?: string; // criterion_id being commented on
}

export interface WSPresenceEntry {
  user_id: string;
  user_name: string;
  patrol_id: string;
  commenting_on?: string;
}

export interface WSPresenceStatePayload {
  session_id: string;
  users: WSPresenceEntry[];
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
  subcamp_id: string;
  subcamp_name: string;
  patrols: WSPatrolProgressPayload[];
}

export interface WSProgressUpdatedPayload {
  session_id: string;
  users: WSUserProgressPayload[];
}
