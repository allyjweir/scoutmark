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
