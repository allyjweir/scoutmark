import type { User, Session, Patrol, Draft, Submission, SubmissionScore, Award, PreviousPatrolTotal, Criterion } from './types';

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

export interface PatrolHistoryComment {
  id: string;
  display_name: string;
  comment: string;
}

export interface PatrolHistoryScore {
  criterion_id: string;
  criterion_title: string;
  min_value: number;
  max_value: number;
  value: number;
  comments: PatrolHistoryComment[];
}

export interface PatrolHistorySession {
  id: string;
  name: string;
  starts_at: string;
  submitted_at: string;
  total: number;
  scores: PatrolHistoryScore[];
}

export interface PatrolHistory {
  patrol_id: string;
  name: string;
  sort_order: number;
  sessions: PatrolHistorySession[];
}

export const getPatrolHistory = async (): Promise<{ patrols: PatrolHistory[] }> =>
  request('/patrols/history');

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
  status: 'not_started' | 'drafting' | 'complete' | 'submitted';
}

export interface UserProgress {
  user_id: string;
  display_name: string;
  subcamp_id: string;
  subcamp_name: string;
  patrols: PatrolProgress[];
  awards?: UserAward[];
}

export interface SubcampProgress {
  subcamp_id: string;
  subcamp_name: string;
  users: UserProgress[];
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
): Promise<{ session: Session; users: UserProgress[]; subcamps?: SubcampProgress[] }> =>
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
  scores: { criterion_id: string; value: number }[];
  comments?: AdminPerUserComment[];
}

export interface AdminUserScoresResponse {
  user_id: string;
  display_name: string;
  session_name: string;
  criteria: Criterion[];
  patrols: AdminPatrolScores[];
}

export const getAdminUserScores = async (
  sessionId: string,
  userId: string,
): Promise<AdminUserScoresResponse> =>
  request(`/admin/sessions/${sessionId}/users/${userId}/scores`);

export const lockSession = async (
  sessionId: string,
): Promise<{ ok: boolean }> =>
  request(`/admin/sessions/${sessionId}/lock`, {
    method: 'POST',
  });

export const unlockSession = async (
  sessionId: string,
): Promise<{ ok: boolean }> =>
  request(`/admin/sessions/${sessionId}/unlock`, {
    method: 'POST',
  });

export interface Round2Finalist {
  subcamp_id: string;
  subcamp_name: string;
  patrol_id: string;
  patrol_name: string;
  selection_source: string;
}

export const ensureRound2 = async (
  sessionId: string,
): Promise<{ session: Session }> =>
  request(`/admin/sessions/${sessionId}/round2`, {
    method: 'POST',
  });

export const getRound2Finalists = async (
  sessionId: string,
): Promise<{ finalists: Round2Finalist[] }> =>
  request(`/admin/sessions/${sessionId}/round2/finalists`);

export const setRound2Finalist = async (
  sessionId: string,
  subcampId: string,
  patrolId: string,
): Promise<{ ok: boolean }> =>
  request(`/admin/sessions/${sessionId}/round2/finalists/${subcampId}`, {
    method: 'PUT',
    body: JSON.stringify({ patrol_id: patrolId }),
  });

export interface AdminSubcamp {
  id: string;
  name: string;
  locked_at?: string | null;
  locked_by?: string | null;
}

export interface AdminUser {
  id: string;
  username: string;
  display_name: string;
  is_admin: boolean;
  subcamp_id: string | null;
  subcamp_name: string | null;
}

export const updateAdminSession = async (
  sessionId: string,
  startsAt: string,
  endsAt: string,
): Promise<{ session: Session }> =>
  request(`/admin/sessions/${sessionId}`, {
    method: 'PUT',
    body: JSON.stringify({ starts_at: startsAt, ends_at: endsAt }),
  });

export const getAdminSessionSubcamps = async (
  sessionId: string,
): Promise<{ subcamps: AdminSubcamp[] }> =>
  request(`/admin/sessions/${sessionId}/subcamps`);

export const lockAdminSessionSubcamp = async (sessionId: string, subcampId: string): Promise<{ ok: boolean }> =>
  request(`/admin/sessions/${sessionId}/subcamps/${subcampId}/lock`, { method: 'POST' });

export const unlockAdminSessionSubcamp = async (sessionId: string, subcampId: string): Promise<{ ok: boolean }> =>
  request(`/admin/sessions/${sessionId}/subcamps/${subcampId}/unlock`, { method: 'POST' });

export const listAdminUsers = async (): Promise<{ users: AdminUser[] }> => request('/admin/users');
export const listAdminSubcamps = async (): Promise<{ subcamps: AdminSubcamp[] }> => request('/admin/subcamps');

export const createAdminUser = async (user: {
  username: string;
  display_name: string;
  password: string;
  subcamp_id: string;
  is_admin: boolean;
}): Promise<AdminUser> =>
  request('/admin/users', { method: 'POST', body: JSON.stringify(user) });

export const resetAdminUserPassword = async (userId: string, password: string): Promise<{ status: string }> =>
  request(`/admin/users/${userId}/password`, {
    method: 'PUT',
    body: JSON.stringify({ password }),
  });

export const updateAdminPatrolScores = async (
  sessionId: string,
  patrolId: string,
  scores: Record<string, number>,
): Promise<{ ok: boolean }> =>
  request(`/admin/sessions/${sessionId}/patrols/${patrolId}/scores`, {
    method: 'PUT',
    body: JSON.stringify({ scores, confirmed: true }),
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
