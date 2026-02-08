import { useState, useEffect, useMemo } from 'react';
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

  // Sessions that were finalised in the last 30 minutes
  const recentlyFinalised = useMemo(() => {
    try {
      const stored = JSON.parse(localStorage.getItem('finalised_sessions') ?? '{}');
      const now = Date.now();
      const thirtyMin = 30 * 60 * 1000;
      const recent = new Set<string>();

      // Clean up old entries while we're at it
      const cleaned: Record<string, string> = {};
      for (const [id, iso] of Object.entries(stored)) {
        const elapsed = now - new Date(iso as string).getTime();
        if (elapsed < thirtyMin) {
          recent.add(id);
          cleaned[id] = iso as string;
        }
      }
      localStorage.setItem('finalised_sessions', JSON.stringify(cleaned));
      return recent;
    } catch {
      return new Set<string>();
    }
  }, [sessions]);

  useEffect(() => {
    api.listSessions()
      .then(({ sessions }) => setSessions(sessions))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  const grouped = groupBy(sessions, 'status');
  const activeSessions = sortBy(grouped['ACTIVE'] ?? [], 'starts_at');
  const upcomingSessions = sortBy(grouped['UPCOMING'] ?? [], 'starts_at');
  const closedSessions = sortBy(grouped['CLOSED'] ?? [], 'ends_at').reverse();

  const handleSessionClick = (session: Session) => {
    if (session.status === 'ACTIVE' || session.status === 'CLOSED') {
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
              recentlyFinalised={recentlyFinalised.has(session.id)}
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
      {closedSessions.length > 0 && (
        <Box mb={4}>
          <Heading sx={{ fontSize: 2, mb: 2, color: 'fg.muted' }}>
            Closed
          </Heading>
          {closedSessions.slice(0, 5).map((session) => (
            <SessionCard
              key={session.id}
              session={session}
              onClick={() => handleSessionClick(session)}
              recentlyFinalised={recentlyFinalised.has(session.id)}
            />
          ))}
        </Box>
      )}

      {sessions.length === 0 && (
        <Box textAlign="center" py={6}>
          <Text sx={{ color: 'fg.muted', fontSize: 2 }}>
            No sessions available yet.
          </Text>
        </Box>
      )}
    </Box>
  );
};
