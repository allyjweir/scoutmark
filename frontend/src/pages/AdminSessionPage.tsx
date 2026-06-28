import { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, Label,
  ProgressBar, CounterLabel,
} from '@primer/react';
import type { Session, WSServerMessage, WSProgressUpdatedPayload } from '../lib/types';
import * as api from '../lib/api';
import type { UserProgress, SessionComment } from '../lib/api';
import { useSessionSubscription } from '../hooks/useWebSocket';

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

  // Comments — loaded eagerly, refreshed on WS updates
  const [comments, setComments] = useState<SessionComment[]>([]);

  // Per-scorer expanded comments toggle
  const [expandedUsers, setExpandedUsers] = useState<Set<string>>(new Set());

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

  // Fetch comments
  const loadComments = useCallback(async () => {
    if (!sessionId) return;
    try {
      const { comments: c } = await api.getSessionComments(sessionId);
      setComments(c ?? []);
    } catch {
      // ignore
    }
  }, [sessionId]);

  // Initial load — progress + comments in parallel
  useEffect(() => {
    if (!sessionId) return;

    Promise.all([
      api.getSessionProgress(sessionId),
      api.getSessionComments(sessionId).catch(() => ({ comments: [] as SessionComment[] })),
    ])
      .then(([progress, commentsResult]) => {
        setSession(progress.session);
        applyUsers(progress.users, true);
        setComments(commentsResult.comments ?? []);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [sessionId, applyUsers]);

  // Live updates via WebSocket
  const handleWSMessage = useCallback((msg: WSServerMessage) => {
    if (msg.type === 'progress_updated') {
      const payload = msg.payload as WSProgressUpdatedPayload;
      if (payload.session_id === sessionId) {
        applyUsers(payload.users as UserProgress[]);
        loadComments();
      }
    }
  }, [sessionId, applyUsers, loadComments]);

  useSessionSubscription(sessionId, handleWSMessage);

  // Toggle a scorer's comments dropdown
  const toggleUserComments = useCallback((userId: string) => {
    setExpandedUsers((prev) => {
      const next = new Set(prev);
      if (next.has(userId)) next.delete(userId);
      else next.add(userId);
      return next;
    });
  }, []);

  // Group comments by user → patrol
  const commentsByUser = useMemo(() => {
    const map: Record<string, Record<string, { patrolName: string; items: SessionComment[] }>> = {};
    for (const c of comments) {
      if (!map[c.user_id]) map[c.user_id] = {};
      if (!map[c.user_id][c.patrol_id]) {
        map[c.user_id][c.patrol_id] = { patrolName: c.patrol_name, items: [] };
      }
      map[c.user_id][c.patrol_id].items.push(c);
    }
    return map;
  }, [comments]);

  // Comment count per user
  const commentCountByUser = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const c of comments) {
      counts[c.user_id] = (counts[c.user_id] || 0) + 1;
    }
    return counts;
  }, [comments]);

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
          <Button variant="invisible" onClick={() => navigate('/admin')} size="small">
            ← Admin workspace
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
          <Box display="flex" alignItems="center" sx={{ gap: 1 }}>
            <Box
              sx={{
                width: 8, height: 8, borderRadius: '50%',
                bg: 'success.emphasis',
                '@keyframes pulse': {
                  '0%, 100%': { opacity: 1 },
                  '50%': { opacity: 0.4 },
                },
                animation: 'pulse 2s ease-in-out infinite',
              }}
            />
            <Text sx={{ fontSize: 0, color: 'fg.muted' }}>Live</Text>
          </Box>
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
            const userCommentCount = commentCountByUser[user.user_id] || 0;
            const isExpanded = expandedUsers.has(user.user_id);
            const userCommentsByPatrol = commentsByUser[user.user_id] || {};

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
                      <Button
                        variant="invisible"
                        onClick={() => navigate(`/admin/sessions/${sessionId}/scorer/${user.user_id}`)}
                        size="small"
                        sx={{ p: 0, fontWeight: 'bold', fontSize: 2 }}
                      >
                        {user.display_name}
                      </Button>
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

                {/* Award winners (shown when user has finalised and has awards) */}
                {userDone && user.awards && user.awards.length > 0 && (
                  <Box
                    px={3}
                    pb={3}
                    borderTopWidth={1}
                    borderTopStyle="solid"
                    borderTopColor="border.default"
                    pt={2}
                  >
                    <Text sx={{ fontSize: 0, color: 'fg.muted', fontWeight: 'bold', mb: 1, display: 'block' }}>
                      🏆 Awards
                    </Text>
                    <Box display="flex" flexWrap="wrap" sx={{ gap: 2 }}>
                      {user.awards.map((award) => (
                        <Box
                          key={award.award_type}
                          display="flex"
                          alignItems="center"
                          sx={{ gap: 1 }}
                        >
                          <Text sx={{ fontSize: 0 }}>
                            {award.award_type === 'best_patrol' ? '🥇' : '📈'}
                          </Text>
                          <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
                            {award.award_type === 'best_patrol' ? 'Best:' : 'Most Improved:'}
                          </Text>
                          <Label variant="accent" size="small">
                            {award.patrol_name || award.patrol_id}
                          </Label>
                        </Box>
                      ))}
                    </Box>
                  </Box>
                )}

                {/* Per-scorer comments dropdown */}
                {userCommentCount > 0 && (
                  <Box
                    borderTopWidth={1}
                    borderTopStyle="solid"
                    borderTopColor="border.default"
                  >
                    <Box
                      as="button"
                      onClick={() => toggleUserComments(user.user_id)}
                      sx={{
                        width: '100%',
                        px: 3,
                        py: 2,
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'center',
                        bg: 'transparent',
                        border: 'none',
                        cursor: 'pointer',
                        ':hover': { bg: 'canvas.subtle' },
                      }}
                    >
                      <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                        <Text sx={{ fontSize: 1, fontWeight: 'bold' }}>💬 Comments</Text>
                        <CounterLabel>{userCommentCount}</CounterLabel>
                      </Box>
                      <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
                        {isExpanded ? '▲' : '▼'}
                      </Text>
                    </Box>

                    {isExpanded && (
                      <Box px={3} pb={3}>
                        <Box display="flex" flexDirection="column" sx={{ gap: 3 }}>
                          {Object.entries(userCommentsByPatrol).map(([patrolId, { patrolName, items }]) => (
                            <Box key={patrolId}>
                              <Box display="flex" alignItems="center" sx={{ gap: 2 }} mb={2}>
                                <Text sx={{ fontWeight: 'bold', fontSize: 1 }}>
                                  {patrolName}
                                </Text>
                                <CounterLabel>{items.length}</CounterLabel>
                              </Box>

                              <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
                                {items.map((c, i) => (
                                  <Box
                                    key={`${c.criterion_id}-${i}`}
                                    p={2}
                                    borderWidth={1}
                                    borderStyle="solid"
                                    borderColor="border.default"
                                    borderRadius={2}
                                    bg="canvas.subtle"
                                  >
                                    <Box display="flex" justifyContent="space-between" alignItems="baseline" mb={1}>
                                      <Text sx={{ fontSize: 0, fontWeight: 'bold', color: 'fg.default' }}>
                                        {c.criterion_title}
                                      </Text>
                                      <Text sx={{ fontSize: 0, color: 'fg.muted', fontVariantNumeric: 'tabular-nums' }}>
                                        Score: {c.value}/10
                                      </Text>
                                    </Box>
                                    <Text sx={{ fontSize: 1, display: 'block', color: 'fg.default' }}>
                                      &ldquo;{c.comment}&rdquo;
                                    </Text>
                                  </Box>
                                ))}
                              </Box>
                            </Box>
                          ))}
                        </Box>
                      </Box>
                    )}
                  </Box>
                )}
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
