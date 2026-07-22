import { useState, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { Box, Heading, Text, Spinner, Flash, Button, ConfirmationDialog } from '@primer/react';
import { groupBy, sortBy } from 'lodash';
import type { Session } from '../lib/types';
import * as api from '../lib/api';
import { useAuth } from '../hooks/useAuth';
import { SessionCard } from '../components/SessionCard';

export const DashboardPage = () => {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [finalisingSession, setFinalisingSession] = useState<Session | null>(null);
  const [completingFinalising, setCompletingFinalising] = useState(false);
  const [completionMessage, setCompletionMessage] = useState('');

  // Success flash from finalise navigation
  const finalisedName = (location.state as { finalised?: string } | null)?.finalised;
  const [showSuccess, setShowSuccess] = useState(!!finalisedName);

  // Clear the navigation state so it doesn't persist on refresh
  useEffect(() => {
    if (finalisedName) {
      window.history.replaceState({}, '');
      const timer = setTimeout(() => setShowSuccess(false), 5000);
      return () => clearTimeout(timer);
    }
  }, [finalisedName]);

  useEffect(() => {
    api.listSessions()
      .then(({ sessions }) => setSessions(sessions))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  const isRegularSession = (session: Session) => (session.round_type ?? 'regular') === 'regular';
  const isCampChiefAccount = user?.is_camp_chief === true;
  const visibleSessions = user?.is_admin ? sessions : sessions.filter(isRegularSession);
  const sessionsById = sessions.reduce<Record<string, Session>>((acc, session) => {
    acc[session.id] = session;
    return acc;
  }, {});
  const round2BySource = sessions
    .filter((s) => (s.round_type ?? 'regular') === 'round2' && s.source_session_id)
    .reduce<Record<string, Session>>((acc, session) => {
      acc[session.source_session_id as string] = session;
      return acc;
    }, {});

  const RECENT_WINNER_WINDOW_MS = 6 * 60 * 60 * 1000;
  const recentRound2Winner = sessions
    .filter((s) => {
      if ((s.round_type ?? 'regular') !== 'round2') return false;
      if (!s.locked_at) return false;
      if (!s.winner_patrol_name || !s.winner_subcamp_name) return false;
      const lockedAtMs = new Date(s.locked_at).getTime();
      if (Number.isNaN(lockedAtMs)) return false;
      const ageMs = Date.now() - lockedAtMs;
      return ageMs >= 0 && ageMs <= RECENT_WINNER_WINDOW_MS;
    })
    .sort((a, b) => new Date(b.locked_at as string).getTime() - new Date(a.locked_at as string).getTime())[0];
  const recentWinnerSourceSession = recentRound2Winner?.source_session_id
    ? sessionsById[recentRound2Winner.source_session_id]
    : undefined;
  const recentWinnerDateLabel = recentWinnerSourceSession?.name
    ?? (recentRound2Winner?.locked_at
      ? new Date(recentRound2Winner.locked_at).toLocaleDateString(undefined, {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
      })
      : 'today');

  const leaderNoteFor = (session: Session): string | undefined => {
    if (user?.is_admin) return undefined;
    const linkedRound2 = round2BySource[session.id];
    if (!linkedRound2) return undefined;

    if (linkedRound2.status === 'ACTIVE' || linkedRound2.status === 'UPCOMING') {
      return 'Camp Chief is selecting overall best patrol.';
    }

    if (linkedRound2.status === 'LOCKED' || linkedRound2.status === 'CLOSED') {
      if (linkedRound2.winner_patrol_name && linkedRound2.winner_subcamp_name) {
        return `Overall winner: ${linkedRound2.winner_subcamp_name} - ${linkedRound2.winner_patrol_name}.`;
      }
      return 'Camp Chief has finalised overall scoring.';
    }

    return undefined;
  };

  const isFinalising = (session: Session): boolean => {
    if ((session.round_type ?? 'regular') !== 'regular') return false;
    const linkedRound2 = round2BySource[session.id];
    if (!linkedRound2) return false;
    return linkedRound2.status === 'ACTIVE' || linkedRound2.status === 'UPCOMING';
  };

  const hasOverallWinnerSelected = (session: Session): boolean => {
    const linkedRound2 = round2BySource[session.id];
    if (!linkedRound2) return false;
    if (linkedRound2.status !== 'LOCKED' && linkedRound2.status !== 'CLOSED') return false;
    return Boolean(linkedRound2.winner_patrol_name && linkedRound2.winner_subcamp_name);
  };

  const isFinalisingComplete = (session: Session): boolean => {
    const linkedRound2 = round2BySource[session.id];
    return linkedRound2?.status === 'LOCKED' || linkedRound2?.status === 'CLOSED';
  };

  const finalisingSessions = sortBy(visibleSessions.filter(isFinalising), 'ends_at').reverse();

  const grouped = groupBy(visibleSessions.filter((session) => !isFinalising(session)), 'status');
  const activeSessions = sortBy(grouped['ACTIVE'] ?? [], 'starts_at');
  const upcomingSessions = sortBy(grouped['UPCOMING'] ?? [], 'starts_at');
  const { completedFinalisingSessions, lockedSessions: lockedSessionsNotCompleted } = (grouped['LOCKED'] ?? []).reduce<{
    completedFinalisingSessions: Session[];
    lockedSessions: Session[];
  }>((result, session) => {
    if (isFinalisingComplete(session)) {
      result.completedFinalisingSessions.push(session);
    } else {
      result.lockedSessions.push(session);
    }
    return result;
  }, { completedFinalisingSessions: [], lockedSessions: [] });
  const lockedSessions = sortBy(lockedSessionsNotCompleted, 'starts_at');
  const closedSessions = sortBy([...(grouped['CLOSED'] ?? []), ...completedFinalisingSessions], 'ends_at').reverse();

  const handleCompleteFinalising = async () => {
    if (!finalisingSession) return;
    setCompletingFinalising(true);
    setError('');
    setCompletionMessage('');
    try {
      const { source_session: sourceSession, round2_session: round2Session } = await api.completeFinalisingSession(finalisingSession.id);
      setSessions((current) => current.map((session) => (
        session.id === sourceSession.id ? sourceSession
          : session.id === round2Session.id ? round2Session
            : session
      )));
      setCompletionMessage(`${finalisingSession.name} has been moved to Closed.`);
      setFinalisingSession(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not complete finalising.');
    } finally {
      setCompletingFinalising(false);
    }
  };

  const handleSessionClick = (session: Session) => {
    if (session.status === 'ACTIVE' || session.status === 'LOCKED' || session.status === 'CLOSED') {
      if (user?.is_admin) {
        if (isCampChiefAccount && (session.round_type ?? 'regular') === 'round2') {
          navigate(`/campchief/sessions/${session.id}`);
          return;
        }
        navigate(`/admin/sessions/${session.id}`);
        return;
      }
      if (isCampChiefAccount && (session.round_type ?? 'regular') === 'round2') {
        navigate(`/campchief/sessions/${session.id}`);
        return;
      }
      navigate(`/sessions/${session.id}`);
    }
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
      </Box>
    );
  }

  return (
    <Box p={3} maxWidth="600px" mx="auto">
      {/* Header */}
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={4}>
        <Box>
          <Heading sx={{ fontSize: 3 }}>⚜️ Scoutmark</Heading>
          <Text sx={{ color: 'fg.muted', fontSize: 1 }}>{user?.display_name}</Text>
        </Box>
        <Button variant="invisible" onClick={logout} size="small">
          Sign out
        </Button>
      </Box>

      {error && (
        <Flash variant="danger" sx={{ mb: 3 }}>
          {error}
        </Flash>
      )}

      {showSuccess && finalisedName && (
        <Flash variant="success" sx={{ mb: 3 }}>
          🎉 Scores for <strong>{finalisedName}</strong> submitted successfully!
        </Flash>
      )}

      {completionMessage && (
        <Flash variant="success" sx={{ mb: 3 }}>
          {completionMessage}
        </Flash>
      )}

      {recentRound2Winner && (
        <Flash variant="success" sx={{ mb: 3, py: 2 }}>
          🏆 Overall best patrol for <strong>{recentWinnerDateLabel}</strong>: <strong>{recentRound2Winner.winner_subcamp_name} - {recentRound2Winner.winner_patrol_name}</strong>
        </Flash>
      )}

      <Box mb={4}>
        <Button onClick={() => navigate('/patrols')} sx={{ width: '100%', justifyContent: 'space-between' }}>
          <span>View patrols &amp; score history</span>
          <span>›</span>
        </Button>
      </Box>

      {/* Admin quick-access */}
      {user?.is_admin && !isCampChiefAccount && (
        <Box mb={4} p={3} borderWidth={1} borderStyle="solid" borderColor="accent.emphasis" borderRadius={2} bg="accent.subtle">
          <Heading sx={{ fontSize: 1, mb: 2, color: 'accent.fg' }}>
            🛡️ Admin
          </Heading>
          <Button onClick={() => navigate('/admin')}>Open admin dashboard</Button>
        </Box>
      )}

      {/* Active Sessions */}
      {activeSessions.length > 0 && (
        <Box mb={4}>
          <Heading sx={{ fontSize: 2, mb: 2, color: 'success.fg' }}>
            Active now
          </Heading>
          {activeSessions.map((session) => (
            <SessionCard
              key={session.id}
              session={session}
              onClick={() => handleSessionClick(session)}
              recentlyFinalised={session.user_finalised}
              note={leaderNoteFor(session)}
            />
          ))}
        </Box>
      )}

      {/* Upcoming Sessions */}
      {finalisingSessions.length > 0 && (
        <Box mb={4}>
          <Heading sx={{ fontSize: 2, mb: 2, color: 'attention.fg' }}>
            Finalising
          </Heading>
          {finalisingSessions.map((session) => (
            <Box key={session.id}>
              <SessionCard
                session={session}
                onClick={() => handleSessionClick(session)}
                recentlyFinalised={session.user_finalised}
                note={leaderNoteFor(session)}
              />
              {user?.is_admin && (
                <Box display="flex" justifyContent="flex-end" mt={-1} mb={2} px={1}>
                  <Button
                    variant="danger"
                    size="small"
                    onClick={() => setFinalisingSession(session)}
                  >
                    Mark Finalising Complete
                  </Button>
                </Box>
              )}
            </Box>
          ))}
        </Box>
      )}

      {/* Upcoming Sessions */}
      {upcomingSessions.length > 0 && (
        <Box mb={4}>
          <Heading sx={{ fontSize: 2, mb: 2, color: 'fg.muted' }}>
            Upcoming
          </Heading>
          {upcomingSessions.map((session) => (
            <SessionCard key={session.id} session={session} disabled />
          ))}
        </Box>
      )}

      {/* Closed Sessions */}
      {lockedSessions.length > 0 && (
        <Box mb={4}>
          <Heading sx={{ fontSize: 2, mb: 2, color: 'danger.fg' }}>
            Locked
          </Heading>
          {lockedSessions.map((session) => (
            <SessionCard
              key={session.id}
              session={session}
              onClick={() => handleSessionClick(session)}
              recentlyFinalised={session.user_finalised}
              note={leaderNoteFor(session)}
            />
          ))}
        </Box>
      )}

      {/* Closed Sessions */}
      {closedSessions.length > 0 && (
        <Box mb={4}>
          <Heading sx={{ fontSize: 2, mb: 2, color: 'fg.muted' }}>
            Closed
          </Heading>
          {closedSessions.slice(0, 5).map((session) => (
            <Box key={session.id}>
              <SessionCard
                session={session}
                onClick={() => handleSessionClick(session)}
                recentlyFinalised={session.user_finalised && !hasOverallWinnerSelected(session)}
                note={leaderNoteFor(session)}
              />
              <Box display="flex" justifyContent="flex-end" mt={-1} mb={2} px={1}>
                <Button
                  as="a"
                  href={`/api/sessions/${session.id}/report-card`}
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="invisible"
                  size="small"
                  sx={{ fontSize: 0, color: 'fg.muted' }}
                >
                  🖨️ Printable Summary
                </Button>
              </Box>
            </Box>
          ))}
        </Box>
      )}

      {visibleSessions.length === 0 && (
        <Box textAlign="center" py={6}>
          <Text sx={{ color: 'fg.muted', fontSize: 2 }}>
            No sessions available yet.
          </Text>
        </Box>
      )}
      {finalisingSession && (
        <ConfirmationDialog
          title="Mark Finalising Complete?"
          confirmButtonContent={completingFinalising ? 'Completing...' : 'Mark Finalising Complete'}
          confirmButtonType="danger"
          onClose={(gesture) => {
            if (gesture === 'confirm') {
              void handleCompleteFinalising();
            } else if (!completingFinalising) {
              setFinalisingSession(null);
            }
          }}
        >
          This will lock the linked Round 2 session and move this session out of Finalising into Closed.
        </ConfirmationDialog>
      )}
    </Box>
  );
};
