import { useState, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { Box, Heading, Text, Spinner, Flash, Button } from '@primer/react';
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

  // Success flash from finalise navigation
  const finalisedState = location.state as { finalised?: string; finalisedSessionId?: string } | null;
  const finalisedName = finalisedState?.finalised;
  const finalisedSessionId = finalisedState?.finalisedSessionId;
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
  const visibleSessions = isCampChiefAccount
    ? sessions.filter((session) => session.round_type === 'round2')
    : sessions.filter(isRegularSession);
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
  const grouped = groupBy(visibleSessions, 'status');
  const activeSessions = sortBy(grouped['ACTIVE'] ?? [], 'starts_at');
  const upcomingSessions = sortBy(grouped['UPCOMING'] ?? [], 'starts_at');
  const lockedSessions = sortBy(grouped['LOCKED'] ?? [], 'starts_at');
  const closedSessions = sortBy(grouped['CLOSED'] ?? [], 'ends_at').reverse();

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
          {finalisedSessionId && (
            <Button
              as="a"
              href={`/api/sessions/${finalisedSessionId}/report-card`}
              target="_blank"
              rel="noopener noreferrer"
              size="small"
              sx={{ ml: 2 }}
            >
              🖨️ Printable Summary
            </Button>
          )}
        </Flash>
      )}

      {recentRound2Winner && (
        <Flash variant="success" sx={{ mb: 3, py: 2 }}>
          🏆 Overall best patrol: <strong>{recentRound2Winner.winner_subcamp_name} - {recentRound2Winner.winner_patrol_name}</strong>
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
            />
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
                recentlyFinalised={session.user_finalised}
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
    </Box>
  );
};
