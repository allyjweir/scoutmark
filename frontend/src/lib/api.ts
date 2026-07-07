import type { User, Session, Patrol, Draft, Submission, SubmissionScore, Award, PreviousPatrolTotal } from './types';

const BASE_URL = '/api';

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

const getToken = (): string | null =>
  localStorage.getItem('session_token');

const request = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  const token = getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...(options.headers as Record<string, string> ?? {}),
  };

  const response = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
    credentials: 'include',
  });

  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: 'Unknown error' }));
    throw new ApiError(response.status, body.error ?? 'Request failed');
  }

  return response.json();
};

// ─── Auth ───────────────────────────────────────────────────────────

export const login = async (username: string, password: string): Promise<{ session_token: string; user: User; password_change_required: boolean }> => {
  const result = await request<{ session_token: string; user: User; password_change_required: boolean }>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  localStorage.setItem('session_token', result.session_token);
  return result;
};

export const logout = async (): Promise<void> => {
  await request('/auth/logout', { method: 'POST' });
  localStorage.removeItem('session_token');
};

export const getCurrentUser = async (): Promise<User> =>
  request<User>('/auth/me');

export const changePassword = async (newPassword: string): Promise<{ status: string }> =>
  request('/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ new_password: newPassword }),
  });

// ─── Sessions ───────────────────────────────────────────────────────

export const listSessions = async (statuses?: string[]): Promise<{ sessions: Session[] }> => {
  const params = statuses?.length
    ? '?' + statuses.map(s => `status=${s}`).join('&')
    : '';
  return request<{ sessions: Session[] }>(`/sessions${params}`);
};

export const getSession = async (sessionId: string): Promise<{
  session: Session;
  patrols: Patrol[];
  submissions: Submission[];
  awards: Award[];
}> => request(`/sessions/${sessionId}`);

// ─── Drafts ─────────────────────────────────────────────────────────

export const getDraft = async (sessionId: string, patrolId: string): Promise<{ draft: Draft | null }> =>
  request(`/sessions/${sessionId}/patrols/${patrolId}/draft`);

// ─── Submissions ────────────────────────────────────────────────────

export const submitScores = async (
  sessionId: string,
  patrolId: string,
  scores: Record<string, number>,
): Promise<Submission> =>
  request(`/sessions/${sessionId}/patrols/${patrolId}/submit`, {
    method: 'POST',
    body: JSON.stringify({ scores }),
  });

export const finaliseSession = async (
  sessionId: string,
): Promise<{ submissions: Submission[]; finalised_count: number }> =>
  request(`/sessions/${sessionId}/finalise`, {
    method: 'POST',
  });

export const listSubmissions = async (sessionId: string): Promise<{ submissions: Submission[] }> =>
  request(`/sessions/${sessionId}/submissions`);

export const reviseSession = async (
  sessionId: string,
): Promise<{ ok: boolean }> =>
  request(`/sessions/${sessionId}/revise`, {
    method: 'POST',
  });

export const getSubmissionScores = async (
  sessionId: string,
  patrolId: string,
): Promise<{ scores: SubmissionScore[] }> =>
  request(`/sessions/${sessionId}/patrols/${patrolId}/scores`);

export const unlockSubmission = async (submissionId: string): Promise<Submission> =>
  request(`/submissions/${submissionId}/unlock`, { method: 'POST' });

// ─── Awards ─────────────────────────────────────────────────────────

export const saveAward = async (
  sessionId: string,
  awardType: string,
  patrolId: string,
): Promise<Award> =>
  request(`/sessions/${sessionId}/awards`, {
    method: 'POST',
    body: JSON.stringify({ award_type: awardType, patrol_id: patrolId }),
  });

export const getPreviousScores = async (
  sessionId: string,
): Promise<{ totals: PreviousPatrolTotal[] }> =>
  request(`/sessions/${sessionId}/previous-scores`);

// ─── Admin ──────────────────────────────────────────────────────────

export interface UserAward {
  award_type: string;
  patrol_id: string;
  patrol_name: string;
}

export interface PatrolProgress {
  patrol_id: string;
  patrol_name: string;
  status: 'not_started' | 'drafting' | 'submitted';
}

export interface UserProgress {
  user_id: string;
  display_name: string;
  patrols: PatrolProgress[];
  awards?: UserAward[];
}

export interface SessionComment {
  user_id: string;
  display_name: string;
  patrol_id: string;
  patrol_name: string;
  criterion_id: string;
  criterion_title: string;
  value: number;
  comment: string;
}

export const getSessionProgress = async (
  sessionId: string,
): Promise<{ session: Session; users: UserProgress[] }> =>
  request(`/admin/sessions/${sessionId}/progress`);

export const getSessionComments = async (
  sessionId: string,
): Promise<{ comments: SessionComment[] }> =>
  request(`/admin/sessions/${sessionId}/comments`);

export interface AdminPerUserComment {
  criterion_id: string;
  user_id: string;
  display_name: string;
  comment: string;
}

export interface AdminPatrolScores {
  patrol_id: string;
  patrol_name: string;
  scores: { criterion_id: string; value: number; comment: string }[];
  comments?: AdminPerUserComment[];
}

export interface AdminUserScoresResponse {
  user_id: string;
  display_name: string;
  session_name: string;
  criteria: { id: string; title: string; description: string; min_value: number; max_value: number; sort_order: number }[];
  patrols: AdminPatrolScores[];
}

export const getAdminUserScores = async (
  sessionId: string,
  userId: string,
): Promise<AdminUserScoresResponse> =>
  request(`/admin/sessions/${sessionId}/users/${userId}/scores`);

export interface AdminUserPatrol {
  patrol_id: string;
  patrol_name: string;
  sort_order: number;
}

export interface AdminUserSummary {
  id: string;
  username: string;
  display_name: string;
  is_admin: boolean;
  created_at: string;
  patrols: AdminUserPatrol[];
}

export interface AdminEventSummary {
  id: string;
  name: string;
  description: string;
  created_at: string;
}

export interface AdminCriterionSummary {
  id: string;
  title: string;
  description: string;
  min_value: number;
  max_value: number;
  sort_order: number;
}

export interface AdminTemplateSummary {
  id: string;
  name: string;
  description: string;
  created_at: string;
  criteria: AdminCriterionSummary[];
}

export interface AdminPatrolSummary {
  id: string;
  name: string;
  created_at: string;
}

export interface AdminSessionSummary {
  id: string;
  event_id: string;
  event_name: string;
  template_id: string;
  template_name: string;
  name: string;
  starts_at: string;
  ends_at: string;
  status: 'UPCOMING' | 'ACTIVE' | 'CLOSED';
  created_at: string;
  previous_session_id: string | null;
  award_best_patrol: boolean;
  award_most_improved: boolean;
}

export interface AdminBootstrap {
  users: AdminUserSummary[];
  events: AdminEventSummary[];
  templates: AdminTemplateSummary[];
  patrols: AdminPatrolSummary[];
  sessions: AdminSessionSummary[];
}

export const getAdminBootstrap = async (): Promise<AdminBootstrap> =>
  request('/admin/bootstrap');

export const createAdminUser = async (payload: {
  id?: string;
  username: string;
  password: string;
  display_name: string;
  is_admin: boolean;
}): Promise<{ ok: boolean; id: string }> =>
  request('/admin/users', {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const changeAdminUserPassword = async (
  userId: string,
  password: string,
): Promise<{ ok: boolean }> =>
  request(`/admin/users/${userId}/password`, {
    method: 'PUT',
    body: JSON.stringify({ password }),
  });

export const createAdminEvent = async (payload: {
  id?: string;
  name: string;
  description: string;
}): Promise<{ ok: boolean; id: string }> =>
  request('/admin/events', {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const createAdminTemplate = async (payload: {
  id?: string;
  name: string;
  description: string;
}): Promise<{ ok: boolean; id: string }> =>
  request('/admin/templates', {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const addAdminCriterion = async (
  templateId: string,
  payload: {
    id?: string;
    title: string;
    description: string;
    min_value: number;
    max_value: number;
    sort_order: number;
  },
): Promise<{ ok: boolean; id: string; template_name: string }> =>
  request(`/admin/templates/${templateId}/criteria`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const createAdminPatrol = async (payload: {
  id?: string;
  name: string;
}): Promise<{ ok: boolean; id: string }> =>
  request('/admin/patrols', {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const assignAdminPatrol = async (
  userId: string,
  payload: {
    patrol_id: string;
    sort_order: number;
  },
): Promise<{ ok: boolean; user: string; patrol: string }> =>
  request(`/admin/users/${userId}/patrols`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const createAdminSession = async (payload: {
  id?: string;
  event_id: string;
  template_id: string;
  name: string;
  start: string;
  duration: string;
  award_best_patrol: boolean;
  award_most_improved: boolean;
  previous_session_id: string;
}): Promise<{ ok: boolean; id: string; event_name: string; template_name: string }> =>
  request('/admin/sessions', {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const updateAdminSession = async (
  sessionId: string,
  payload: {
    award_best_patrol: boolean;
    award_most_improved: boolean;
    previous_session_id: string;
  },
): Promise<{ ok: boolean }> =>
  request(`/admin/sessions/${sessionId}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });

export const seedAdminSessionScores = async (
  sessionId: string,
  payload: {
    user_id: string;
    min_score: number;
    max_score: number;
  },
): Promise<{ ok: boolean; seeded: number }> =>
  request(`/admin/sessions/${sessionId}/seed-scores`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });

// ─── Per-User Comments ──────────────────────────────────────────────

export interface DraftCommentAPI {
  criterion_id: string;
  user_id: string;
  display_name: string;
  comment: string;
  updated_at: string;
}

export const getDraftComments = async (
  sessionId: string,
  patrolId: string,
): Promise<{ comments: DraftCommentAPI[] }> =>
  request(`/sessions/${sessionId}/patrols/${patrolId}/comments`);

export const saveDraftComment = async (
  sessionId: string,
  patrolId: string,
  criterionId: string,
  comment: string,
): Promise<DraftCommentAPI> =>
  request(`/sessions/${sessionId}/patrols/${patrolId}/comments/${criterionId}`, {
    method: 'PUT',
    body: JSON.stringify({ comment }),
  });

export const deleteDraftComment = async (
  sessionId: string,
  patrolId: string,
  criterionId: string,
): Promise<{ status: string }> =>
  request(`/sessions/${sessionId}/patrols/${patrolId}/comments/${criterionId}`, {
    method: 'DELETE',
  });

export const getSubmittedComments = async (
  sessionId: string,
  patrolId: string,
): Promise<{ comments: DraftCommentAPI[] }> =>
  request(`/sessions/${sessionId}/patrols/${patrolId}/submitted-comments`);
