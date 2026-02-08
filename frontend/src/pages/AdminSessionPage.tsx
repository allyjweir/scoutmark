import { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, Label,
  ProgressBar, CounterLabel,
} from '@primer/react';
import type { Session } from '../lib/types';
import * as api from '../lib/api';
import type { UserProgress } from '../lib/api';

const STATUS_COLORS: Record<string, string> = {
  submitted: 'success.emphasis',
  drafting: 'attention.emphasis',
  not_started: 'neutral.muted',
};

const STATUS_LABELS: Record<string, string> = {
  submitted: '✓',
  drafting: '…',
  not_started: '–',
};

export const AdminSessionPage = () => {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();

  const [session, setSession] = useState<Session | null>(null);
  const [users, setUsers] = useState<UserProgress[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Track which users changed on the last refresh
  const [changedUserIds, setChangedUserIds] = useState<Set<string>>(new Set());
  const prevSnapshotRef = useRef<Record<string, string>>({});

  const snapshotUsers = (u: UserProgress[]) => {
    const snap: Record<string, string> = {};
    for (const user of u) {
      snap[user.user_id] = user.patrols.map((p) => `${p.patrol_id}:${p.status}`).join(',');
    }
    return snap;
  };

  const applyUsers = useCallback((incoming: UserProgress[], isInitial = false) => {
    if (!isInitial) {
      const prev = prevSnapshotRef.current;
      const next = snapshotUsers(incoming);
      const changed = new Set<string>();
      for (const user of incoming) {
        if (prev[user.user_id] !== next[user.user_id]) {
          changed.add(user.user_id);
        }
      }
      if (changed.size > 0) {
        setChangedUserIds(changed);
        setTimeout(() => setChangedUserIds(new Set()), 2000);
      }
    }
    prevSnapshotRef.current = snapshotUsers(incoming);
    setUsers(incoming);
  }, []);

  useEffect(() => {
    if (!sessionId) return;

    api.getSessionProgress(sessionId)
      .then(({ session, users }) => {
        setSession(session);
        applyUsers(users, true);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [sessionId, applyUsers]);

  // Auto-refresh countdown (15 seconds)
  const REFRESH_INTERVAL = 15;
  const [countdown, setCountdown] = useState(REFRESH_INTERVAL);
  const countdownRef = useRef(REFRESH_INTERVAL);

  const refreshNow = useCallback(() => {
    if (!sessionId) return;
    api.getSessionProgress(sessionId)
      .then(({ users }) => applyUsers(users))
      .catch(() => { /* silent refresh failure */ });
    setCountdown(REFRESH_INTERVAL);
    countdownRef.current = REFRESH_INTERVAL;
  }, [sessionId, applyUsers]);

  useEffect(() => {
    if (!sessionId) return;
    const tick = setInterval(() => {
      countdownRef.current -= 1;
      setCountdown(countdownRef.current);
      if (countdownRef.current <= 0) {
        refreshNow();
      }
    }, 1000);
    return () => clearInterval(tick);
  }, [sessionId, refreshNow]);

  // Overall stats
  const stats = useMemo(() => {
    let totalPatrols = 0;
    let submitted = 0;
    let drafting = 0;

    for (const user of users) {
      for (const patrol of user.patrols) {
        totalPatrols++;
        if (patrol.status === 'submitted') submitted++;
        else if (patrol.status === 'drafting') drafting++;
      }
    }

    return {
      totalPatrols,
      submitted,
      drafting,
      notStarted: totalPatrols - submitted - drafting,
      percentComplete: totalPatrols > 0 ? Math.round((submitted / totalPatrols) * 100) : 0,
    };
  }, [users]);

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
      </Box>
    );
  }

  if (!session) {
    return (
      <Box p={4} textAlign="center">
        <Flash variant="danger">Session not found</Flash>
      </Box>
    );
  }

  return (
    <Box display="flex" flexDirection="column" minHeight="100vh" bg="canvas.default">
      {/* Top bar */}
      <Box
        p={3}
        borderBottomWidth={1}
        borderBottomStyle="solid"
        borderBottomColor="border.default"
        bg="canvas.subtle"
      >
        <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
          <Button variant="invisible" onClick={() => navigate('/')} size="small">
            ← Dashboard
          </Button>
          <Label variant={
            session.status === 'ACTIVE' ? 'success'
            : session.status === 'UPCOMING' ? 'accent'
            : 'default'
          }>
            {session.status}
          </Label>
        </Box>
        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{session.event_name}</Text>
        <Heading sx={{ fontSize: 3, mb: 1 }}>{session.name}</Heading>
        <Box display="flex" justifyContent="space-between" alignItems="center" mt={1}>
          <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
            Admin Progress View
          </Text>
          <Button variant="invisible" size="small" onClick={refreshNow} sx={{ fontSize: 0, color: 'fg.muted', p: 0 }}>
            ↻ {countdown}s
          </Button>
        </Box>
        {/* Refresh countdown bar */}
        <Box mt={2} height="2px" borderRadius={1} bg="neutral.muted" overflow="hidden">
          <Box
            height="100%"
            bg="accent.emphasis"
            sx={{
              width: `${(countdown / REFRESH_INTERVAL) * 100}%`,
              transition: 'width 1s linear',
            }}
          />
        </Box>
      </Box>

      {error && (
        <Flash variant="danger" sx={{ m: 3 }}>
          {error}
        </Flash>
      )}

      {/* Overall progress */}
      <Box p={3} borderBottomWidth={1} borderBottomStyle="solid" borderBottomColor="border.default">
        <Heading sx={{ fontSize: 2, mb: 2 }}>Overall Completion</Heading>
        <Box display="flex" alignItems="center" sx={{ gap: 2 }} mb={2}>
          <ProgressBar
            progress={stats.percentComplete}
            sx={{ flex: 1 }}
          />
          <CounterLabel>{stats.percentComplete}%</CounterLabel>
        </Box>
        <Box display="flex" sx={{ gap: 3 }}>
          <Box display="flex" alignItems="center" sx={{ gap: 1 }}>
            <Box
              sx={{
                width: 10, height: 10, borderRadius: '50%',
                bg: 'success.emphasis',
              }}
            />
            <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
              Submitted: {stats.submitted}
            </Text>
          </Box>
          <Box display="flex" alignItems="center" sx={{ gap: 1 }}>
            <Box
              sx={{
                width: 10, height: 10, borderRadius: '50%',
                bg: 'attention.emphasis',
              }}
            />
            <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
              In progress: {stats.drafting}
            </Text>
          </Box>
          <Box display="flex" alignItems="center" sx={{ gap: 1 }}>
            <Box
              sx={{
                width: 10, height: 10, borderRadius: '50%',
                bg: 'neutral.muted',
              }}
            />
            <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
              Not started: {stats.notStarted}
            </Text>
          </Box>
        </Box>
      </Box>

      {/* Per-user progress */}
      <Box flex={1} p={3} overflow="auto">
        <Heading sx={{ fontSize: 2, mb: 3 }}>
          Scorers ({users.length})
        </Heading>

        <Box display="flex" flexDirection="column" sx={{ gap: 3 }}>
          {users.map((user) => {
            const userSubmitted = user.patrols.filter(
              (p) => p.status === 'submitted',
            ).length;
            const userTotal = user.patrols.length;
            const userDone = userSubmitted === userTotal;

            const justChanged = changedUserIds.has(user.user_id);

            return (
              <Box
                key={user.user_id}
                borderWidth={1}
                borderStyle="solid"
                borderColor={userDone ? 'success.emphasis' : 'border.default'}
                borderRadius={2}
                bg={userDone ? 'success.subtle' : 'canvas.default'}
                overflow="hidden"
                sx={{
                  '@keyframes cardFlash': {
                    '0%': { backgroundColor: 'var(--bgColor-accent-emphasis, #0969da)' },
                    '100%': { backgroundColor: 'transparent' },
                  },
                  animation: justChanged
                    ? 'cardFlash 2s ease-out forwards'
                    : 'none',
                  transition: 'box-shadow 0.6s ease-out',
                  boxShadow: justChanged
                    ? '0 0 0 2px var(--bgColor-accent-emphasis, #0969da)'
                    : 'none',
                }}
              >
                {/* User header */}
                <Box
                  p={3}
                  display="flex"
                  justifyContent="space-between"
                  alignItems="center"
                >
                  <Box>
                    <Text sx={{ fontWeight: 'bold', fontSize: 2 }}>
                      {user.display_name}
                    </Text>
                  </Box>
                  <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                    <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
                      {userSubmitted}/{userTotal}
                    </Text>
                    {userDone ? (
                      <Label variant="success">Complete ✓</Label>
                    ) : userSubmitted > 0 || user.patrols.some((p) => p.status === 'drafting') ? (
                      <Label variant="attention">In Progress</Label>
                    ) : (
                      <Label>Not Started</Label>
                    )}
                  </Box>
                </Box>

                {/* Patrol grid */}
                <Box
                  px={3}
                  pb={3}
                  display="flex"
                  flexWrap="wrap"
                  sx={{ gap: 1 }}
                >
                  {user.patrols.map((patrol) => (
                    <Box
                      key={patrol.patrol_id}
                      title={`${patrol.patrol_name}: ${patrol.status.replace('_', ' ')}`}
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        px: 2,
                        py: 1,
                        borderRadius: 2,
                        fontSize: 0,
                        fontWeight: 'bold',
                        bg: STATUS_COLORS[patrol.status],
                        color: patrol.status === 'not_started' ? 'fg.muted' : 'fg.onEmphasis',
                        minWidth: '60px',
                        textAlign: 'center',
                      }}
                    >
                      <Text sx={{ fontSize: 0, mr: 1, color: 'inherit' }}>
                        {patrol.patrol_name.length > 8
                          ? patrol.patrol_name.slice(0, 8) + '…'
                          : patrol.patrol_name}
                      </Text>
                      <Text sx={{ fontSize: 0, color: 'inherit' }}>
                        {STATUS_LABELS[patrol.status]}
                      </Text>
                    </Box>
                  ))}
                </Box>
              </Box>
            );
          })}
        </Box>

        {users.length === 0 && (
          <Box textAlign="center" py={6}>
            <Text sx={{ color: 'fg.muted', fontSize: 2 }}>
              No users assigned to this session.
            </Text>
          </Box>
        )}
      </Box>
    </Box>
  );
};
