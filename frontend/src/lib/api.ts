import type { User, Session, Patrol, Draft, Submission, SubmissionScore } from './types';

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

export const login = async (username: string, password: string): Promise<{ session_token: string; user: User }> => {
  const result = await request<{ session_token: string; user: User }>('/auth/login', {
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
