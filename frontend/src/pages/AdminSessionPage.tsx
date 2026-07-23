import { useState, useEffect, useMemo, useCallback } from 'react';
import { useParams, useNavigate, useLocation } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, Label,
  ProgressBar, CounterLabel,
} from '@primer/react';
import type {
  Session,
  Patrol,
  WSServerMessage,
  WSProgressUpdatedPayload,
  WSSessionLockedPayload,
  WSSessionUnlockedPayload,
} from '../lib/types';
import * as api from '../lib/api';
import type { UserProgress, Round2Finalist } from '../lib/api';
import { useSessionSubscription } from '../hooks/useWebSocket';
import { SessionStatusBanner } from '../components/SessionStatusBanner';

type PatrolDisplayState = 'incomplete' | 'complete' | 'finalised';

const DISPLAY_STATE_COLORS: Record<PatrolDisplayState, string> = {
  incomplete: 'attention.emphasis',
  complete: 'accent.emphasis',
  finalised: 'success.emphasis',
};

const DISPLAY_STATE_LABELS: Record<PatrolDisplayState, string> = {
  incomplete: '!',
  complete: '✓',
  finalised: '✓',
};

const displayStateForPatrol = (
  overallStatus: 'submitted' | 'complete' | 'drafting' | 'not_started',
): PatrolDisplayState => {
  if (overallStatus === 'submitted') return 'finalised';
  if (overallStatus === 'complete') return 'complete';
  return 'incomplete';
};

const aggregatePatrolStatus = (
  statuses: Array<'submitted' | 'complete' | 'drafting' | 'not_started'>,
): 'submitted' | 'complete' | 'drafting' | 'not_started' => {
  if (statuses.every((s) => s === 'submitted')) return 'submitted';
  if (statuses.every((s) => s === 'submitted' || s === 'complete')) return 'complete';
  if (statuses.some((s) => s === 'submitted' || s === 'complete' || s === 'drafting')) return 'drafting';
  return 'not_started';
};

export const AdminSessionPage = () => {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const isCampChiefView = location.pathname.startsWith('/campchief/sessions/');

  const [session, setSession] = useState<Session | null>(null);
  const [users, setUsers] = useState<UserProgress[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [locking, setLocking] = useState(false);
  const [round2Finalists, setRound2Finalists] = useState<Round2Finalist[]>([]);
  const [loadingFinalists, setLoadingFinalists] = useState(false);
  const [sessionPatrols, setSessionPatrols] = useState<Patrol[]>([]);
  const [savingFinalistSubcampId, setSavingFinalistSubcampId] = useState<string | null>(null);
  const [loadingRound2Board, setLoadingRound2Board] = useState(false);
  const [round2SubmittedPatrolIds, setRound2SubmittedPatrolIds] = useState<Set<string>>(new Set());
  const [round2PatrolTotals, setRound2PatrolTotals] = useState<Record<string, number | null>>({});
  const [round2WinnerPatrolId, setRound2WinnerPatrolId] = useState('');
  const [savingRound2Winner, setSavingRound2Winner] = useState(false);
  const [copiedAnnouncement, setCopiedAnnouncement] = useState(false);
  const applyUsers = useCallback((incoming: UserProgress[]) => {
    setUsers(incoming);
  }, []);

  // Initial load
  useEffect(() => {
    if (!sessionId) return;

    api.getSessionProgress(sessionId)
      .then((progress) => {
        setSession(progress.session);
        applyUsers(progress.users);
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
      }
      return;
    }

    if (msg.type === 'session_locked') {
      const payload = msg.payload as WSSessionLockedPayload;
      if (payload.session_id === sessionId) {
        setSession((prev) => {
          if (!prev) return prev;
          return {
            ...prev,
            status: 'LOCKED',
            locked_at: payload.locked_at,
            locked_by: payload.user_id,
            locked_by_name: payload.user_display_name,
          };
        });
      }
      return;
    }

    if (msg.type === 'session_unlocked') {
      const payload = msg.payload as WSSessionUnlockedPayload;
      if (payload.session_id === sessionId) {
        setSession((prev) => {
          if (!prev) return prev;
          return {
            ...prev,
            status: 'ACTIVE',
            locked_at: undefined,
            locked_by: undefined,
            locked_by_name: undefined,
          };
        });
      }
    }
  }, [sessionId, applyUsers]);

  useSessionSubscription(sessionId, handleWSMessage);

  const subcampProgress = useMemo(() => {
    const groups: Record<string, { subcampName: string; patrols: Record<string, { patrolName: string; statuses: Array<'submitted' | 'complete' | 'drafting' | 'not_started'> }> }> = {};
    for (const user of users) {
      const id = user.subcamp_id || 'unassigned';
      const name = user.subcamp_name || 'Unassigned';
      if (!groups[id]) {
        groups[id] = { subcampName: name, patrols: {} };
      }
      for (const patrol of user.patrols) {
        if (!groups[id].patrols[patrol.patrol_id]) {
          groups[id].patrols[patrol.patrol_id] = { patrolName: patrol.patrol_name, statuses: [] };
        }
        groups[id].patrols[patrol.patrol_id].statuses.push(patrol.status);
      }
    }

    return Object.entries(groups)
      .sort((a, b) => a[1].subcampName.localeCompare(b[1].subcampName))
      .map(([subcampId, group]) => {
        const patrols = Object.entries(group.patrols)
          .map(([patrolId, patrol]) => {
            const statuses = patrol.statuses;
            const overallStatus = aggregatePatrolStatus(statuses);
            return {
              patrolId,
              patrolName: patrol.patrolName,
              overallStatus,
            };
          })
          .sort((a, b) => a.patrolName.localeCompare(b.patrolName));

        const submittedCount = patrols.filter((p) => p.overallStatus === 'submitted').length;
        const completeCount = patrols.filter((p) => p.overallStatus === 'complete').length;
        const draftingCount = patrols.filter((p) => p.overallStatus === 'drafting').length;
        const notStartedCount = patrols.length - submittedCount - completeCount - draftingCount;

        return {
          subcampId,
          subcampName: group.subcampName,
          patrols,
          submittedCount,
          completeCount,
          draftingCount,
          notStartedCount,
        };
      });
  }, [users]);

  // Overall stats
  const stats = useMemo(() => {
    if ((session?.round_type ?? 'regular') !== 'round2') {
      const totalPatrols = subcampProgress.reduce((sum, subcamp) => sum + subcamp.patrols.length, 0);
      const submitted = subcampProgress.reduce((sum, subcamp) => sum + subcamp.submittedCount, 0);
      const complete = subcampProgress.reduce((sum, subcamp) => sum + subcamp.completeCount, 0);
      const drafting = subcampProgress.reduce((sum, subcamp) => sum + subcamp.draftingCount, 0);
      const incomplete = drafting + (totalPatrols - submitted - complete - drafting);
      return {
        totalPatrols,
        submitted,
        complete,
        drafting: incomplete,
        notStarted: 0,
        percentComplete: totalPatrols > 0 ? Math.round((submitted / totalPatrols) * 100) : 0,
      };
    }

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
      complete: 0,
      drafting,
      notStarted: totalPatrols - submitted - drafting,
      percentComplete: totalPatrols > 0 ? Math.round((submitted / totalPatrols) * 100) : 0,
    };
  }, [session?.round_type, subcampProgress, users]);

  const handleLockToggle = useCallback(async () => {
    if (!sessionId || !session) return;
    setLocking(true);
    setError('');

    try {
      if (session.status === 'LOCKED') {
        await api.unlockSession(sessionId);
      } else {
        await api.lockSession(sessionId);
      }

      const progress = await api.getSessionProgress(sessionId);
      setSession(progress.session);
      applyUsers(progress.users);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not update session lock');
    } finally {
      setLocking(false);
    }
  }, [sessionId, session, applyUsers]);

  const handleSetFinalist = useCallback(async (subcampId: string, patrolId: string) => {
    if (!sessionId || !patrolId) return;
    setSavingFinalistSubcampId(subcampId);
    setError('');
    try {
      await api.setRound2Finalist(sessionId, subcampId, patrolId);
      const refreshed = await api.getRound2Finalists(sessionId);
      setRound2Finalists(refreshed.finalists);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not update finalist');
    } finally {
      setSavingFinalistSubcampId(null);
    }
  }, [sessionId]);

  useEffect(() => {
    if (!sessionId || session?.round_type !== 'round2') {
      setRound2Finalists([]);
      setSessionPatrols([]);
      return;
    }

    setLoadingFinalists(true);
    api.getRound2Finalists(sessionId)
      .then(({ finalists }) => setRound2Finalists(finalists))
      .catch(() => setRound2Finalists([]))
      .finally(() => setLoadingFinalists(false));

    api.getRound2CandidatePatrols(sessionId)
      .then(({ patrols }) => setSessionPatrols(patrols))
      .catch(() => setSessionPatrols([]));
  }, [sessionId, session?.round_type]);

  useEffect(() => {
    if (!sessionId || session?.round_type !== 'round2') {
      setRound2SubmittedPatrolIds(new Set());
      setRound2PatrolTotals({});
      setRound2WinnerPatrolId('');
      return;
    }

    setLoadingRound2Board(true);
    api.getSession(sessionId)
      .then(async ({ submissions, awards }) => {
        const submittedIds = new Set(submissions.map((s) => s.patrol_id));
        setRound2SubmittedPatrolIds(submittedIds);

        const winner = awards.find((a) => a.award_type === 'best_patrol');
        setRound2WinnerPatrolId(winner?.patrol_id ?? '');

        const totals = await Promise.all(round2Finalists.map(async (f) => {
          if (!submittedIds.has(f.patrol_id)) {
            return [f.patrol_id, null] as const;
          }
          try {
            const { scores } = await api.getSubmissionScores(sessionId, f.patrol_id);
            const total = scores.reduce((sum, score) => sum + score.value, 0);
            return [f.patrol_id, total] as const;
          } catch {
            return [f.patrol_id, null] as const;
          }
        }));
        setRound2PatrolTotals(Object.fromEntries(totals));
      })
      .catch(() => {
        setRound2SubmittedPatrolIds(new Set());
        setRound2PatrolTotals({});
      })
      .finally(() => setLoadingRound2Board(false));
  }, [sessionId, session?.round_type, round2Finalists]);

  const handleSetRound2Winner = useCallback(async (patrolId: string) => {
    if (!sessionId || !patrolId) return;
    setSavingRound2Winner(true);
    setError('');
    try {
      await api.saveAward(sessionId, 'best_patrol', patrolId);
      setRound2WinnerPatrolId(patrolId);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save winner');
    } finally {
      setSavingRound2Winner(false);
    }
  }, [sessionId]);

  const selectedRound2Winner = useMemo(
    () => round2Finalists.find((f) => f.patrol_id === round2WinnerPatrolId),
    [round2Finalists, round2WinnerPatrolId],
  );

  const round2AnnouncementText = useMemo(() => {
    if (!selectedRound2Winner) return '';
    return `Camp Chief's Pendant Winners: ${selectedRound2Winner.subcamp_name} ${selectedRound2Winner.patrol_name}`;
  }, [selectedRound2Winner]);

  useEffect(() => {
    setCopiedAnnouncement(false);
  }, [round2AnnouncementText]);

  const handleCopyRound2Announcement = useCallback(async () => {
    if (!round2AnnouncementText) return;
    try {
      await navigator.clipboard.writeText(round2AnnouncementText);
      setCopiedAnnouncement(true);
      setTimeout(() => setCopiedAnnouncement(false), 1800);
    } catch {
      setError('Could not copy announcement text');
    }
  }, [round2AnnouncementText]);

  const patrolOptionsBySubcamp = useMemo(() => {
    const grouped: Record<string, Patrol[]> = {};
    for (const patrol of sessionPatrols) {
      if (!patrol.subcamp_id) continue;
      grouped[patrol.subcamp_id] = grouped[patrol.subcamp_id] ?? [];
      grouped[patrol.subcamp_id].push(patrol);
    }
    return grouped;
  }, [sessionPatrols]);

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

  const isRound2 = session.round_type === 'round2';
  const modeLabel = isRound2 ? 'Round 2 (Camp Chief)' : 'Subcamp Scoring Round';
  const progressHeading = isRound2 ? 'Round 2 Scoring Completion' : 'Overall Completion';
  const rosterHeading = isRound2 ? 'Contributors' : 'Subcamp progress';

  if (isCampChiefView && !isRound2) {
    return (
      <Box p={4} textAlign="center">
        <Flash variant="warning" sx={{ mb: 3 }}>
          Camp Chief view is only available for Round 2 sessions.
        </Flash>
        <Button onClick={() => navigate('/')}>Back to Dashboard</Button>
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
            : session.status === 'LOCKED' ? 'danger'
            : 'default'
          }>
            {session.status}
          </Label>
        </Box>
        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{session.event_name}</Text>
        <Heading sx={{ fontSize: 3, mb: 1 }}>{session.name}</Heading>
        <Box display="flex" justifyContent="space-between" alignItems="center" mt={1}>
          <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
            {modeLabel}
          </Text>
          <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
            <Button
              size="small"
              variant={session.status === 'LOCKED' ? 'default' : 'danger'}
              onClick={handleLockToggle}
              disabled={locking || session.status === 'UPCOMING' || session.status === 'CLOSED'}
            >
              {locking ? 'Working...' : session.status === 'LOCKED' ? 'Unlock Session' : 'Lock Session'}
            </Button>
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

      <Box px={3} pt={3}>
        <SessionStatusBanner session={session} />
      </Box>

      {error && (
        <Flash variant="danger" sx={{ m: 3 }}>
          {error}
        </Flash>
      )}

      {/* Overall progress */}
      <Box p={3} borderBottomWidth={1} borderBottomStyle="solid" borderBottomColor="border.default">
        <Heading sx={{ fontSize: 2, mb: 2 }}>{progressHeading}</Heading>
        <Box display="flex" alignItems="center" sx={{ gap: 2 }} mb={2}>
          <ProgressBar
            progress={stats.percentComplete}
            sx={{ flex: 1 }}
          />
          <CounterLabel>{stats.percentComplete}%</CounterLabel>
        </Box>
        {isRound2 && (
          <Text sx={{ fontSize: 0, color: 'fg.muted', mb: 2, display: 'block' }}>
            Finalists configured: {round2Finalists.length} subcamps
          </Text>
        )}
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
              Incomplete: {stats.drafting}
            </Text>
          </Box>
          <Box display="flex" alignItems="center" sx={{ gap: 1 }}>
            <Box
              sx={{
                width: 10, height: 10, borderRadius: '50%',
                bg: 'accent.emphasis',
              }}
            />
            <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
              Complete: {stats.complete}
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

      {session.round_type === 'round2' && (
        <Box p={3} borderBottomWidth={1} borderBottomStyle="solid" borderBottomColor="border.default">
          <Heading sx={{ fontSize: 2, mb: 2 }}>Round 2: Finalists by Subcamp</Heading>
          <Text sx={{ fontSize: 1, color: 'fg.muted', mb: 2, display: 'block' }}>
            Select one finalist patrol from each participating subcamp before scoring begins.
          </Text>
          {loadingFinalists ? (
            <Text sx={{ fontSize: 1, color: 'fg.muted' }}>Loading finalists...</Text>
          ) : (
            <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
              {Object.entries(patrolOptionsBySubcamp).map(([subcampId, options]) => {
                const finalist = round2Finalists.find((item) => item.subcamp_id === subcampId);
                const subcampName = options[0]?.subcamp ?? finalist?.subcamp_name ?? 'Unknown subcamp';
                const locked = session.status === 'LOCKED' || session.status === 'CLOSED';
                return (
                  <Box key={subcampId} display="flex" alignItems="center" sx={{ gap: 2 }}>
                    <Text sx={{ minWidth: '120px', fontSize: 1, fontWeight: 'bold' }}>{subcampName}</Text>
                    <Box sx={{ flex: 1 }}>
                      <select
                        value={finalist?.patrol_id ?? ''}
                        onChange={(e) => handleSetFinalist(subcampId, e.target.value)}
                        disabled={locked || savingFinalistSubcampId === subcampId || options.length === 0}
                        style={{
                          width: '100%',
                          padding: '8px 12px',
                          borderRadius: '6px',
                          border: '1px solid var(--borderColor-default, #d0d7de)',
                          backgroundColor: 'var(--bgColor-default, #fff)',
                          fontSize: '14px',
                        }}
                      >
                        <option value="">Select finalist...</option>
                        {options.map((patrol) => (
                          <option key={patrol.patrol_id} value={patrol.patrol_id}>
                            {patrol.subcamp ? `${patrol.subcamp} - ${patrol.name}` : patrol.name}
                          </option>
                        ))}
                      </select>
                    </Box>
                    {finalist && <Label variant="accent" size="small">{finalist.selection_source}</Label>}
                  </Box>
                );
              })}
              {Object.keys(patrolOptionsBySubcamp).length === 0 && (
                <Text sx={{ fontSize: 1, color: 'fg.muted' }}>No candidate patrols are available for this session.</Text>
              )}
            </Box>
          )}
        </Box>
      )}

      {isRound2 && (
        <Box p={3} borderBottomWidth={1} borderBottomStyle="solid" borderBottomColor="border.default">
          <Heading sx={{ fontSize: 2, mb: 2 }}>Round 2: Select Overall Winner</Heading>
          <Text sx={{ fontSize: 1, color: 'fg.muted', mb: 2, display: 'block' }}>
            Camp Chief chooses the overall winner from these finalists. This is the final decision (no third round).
          </Text>
          {loadingRound2Board ? (
            <Text sx={{ fontSize: 1, color: 'fg.muted' }}>Loading round 2 progress...</Text>
          ) : (
            <>
              <Box display="flex" flexDirection="column" sx={{ gap: 2, mb: 3 }}>
                {round2Finalists.map((finalist) => {
                  const submitted = round2SubmittedPatrolIds.has(finalist.patrol_id);
                  const total = round2PatrolTotals[finalist.patrol_id];
                  return (
                    <Box
                      key={finalist.subcamp_id}
                      display="flex"
                      justifyContent="space-between"
                      alignItems="center"
                      p={2}
                      borderWidth={1}
                      borderStyle="solid"
                      borderColor="border.default"
                      borderRadius={2}
                    >
                      <Box>
                        <Text sx={{ fontWeight: 'bold', fontSize: 1 }}>{finalist.subcamp_name}</Text>
                        <Text sx={{ fontSize: 0, color: 'fg.muted', display: 'block' }}>{finalist.patrol_name}</Text>
                      </Box>
                      <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                        <Label variant={submitted ? 'success' : 'default'}>
                          {submitted ? 'Submitted' : 'Pending'}
                        </Label>
                        <Text sx={{ fontSize: 0, color: 'fg.muted', minWidth: '72px', textAlign: 'right' }}>
                          {total != null ? `${total} pts` : '—'}
                        </Text>
                      </Box>
                    </Box>
                  );
                })}
              </Box>

              <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                <Text sx={{ fontSize: 1, fontWeight: 'bold', minWidth: '140px' }}>Overall Winner</Text>
                <Box sx={{ flex: 1 }}>
                  <select
                    value={round2WinnerPatrolId}
                    onChange={(e) => handleSetRound2Winner(e.target.value)}
                    disabled={savingRound2Winner || session.status === 'LOCKED' || session.status === 'CLOSED'}
                    style={{
                      width: '100%',
                      padding: '8px 12px',
                      borderRadius: '6px',
                      border: '1px solid var(--borderColor-default, #d0d7de)',
                      backgroundColor: 'var(--bgColor-default, #fff)',
                      fontSize: '14px',
                    }}
                  >
                    <option value="">Select overall winner...</option>
                    {round2Finalists.map((finalist) => (
                      <option key={finalist.patrol_id} value={finalist.patrol_id}>
                        {`${finalist.subcamp_name} - ${finalist.patrol_name}`}
                        {round2PatrolTotals[finalist.patrol_id] != null
                          ? ` (${round2PatrolTotals[finalist.patrol_id]} pts)`
                          : ''}
                      </option>
                    ))}
                  </select>
                </Box>
              </Box>
            </>
          )}
        </Box>
      )}

      {isRound2 && (session.status === 'LOCKED' || session.status === 'CLOSED') && selectedRound2Winner && (
        <Box p={3} borderBottomWidth={1} borderBottomStyle="solid" borderBottomColor="border.default">
          <Heading sx={{ fontSize: 2, mb: 2 }}>Final Announcement</Heading>
          <Box
            p={3}
            borderWidth={1}
            borderStyle="solid"
            borderColor="success.emphasis"
            borderRadius={2}
            bg="success.subtle"
          >
            <Text sx={{ fontSize: 1, fontWeight: 'bold', display: 'block', mb: 2 }}>
              {round2AnnouncementText}
            </Text>
            <Button size="small" onClick={handleCopyRound2Announcement}>
              {copiedAnnouncement ? 'Copied ✓' : 'Copy for WhatsApp'}
            </Button>
          </Box>
        </Box>
      )}

      {/* Subcamp progress */}
      {!isRound2 && (
      <Box flex={1} p={3} overflow="auto">
        <Heading sx={{ fontSize: 2, mb: 3 }}>
          {rosterHeading} ({subcampProgress.length})
        </Heading>

        <Box display="flex" flexDirection="column" sx={{ gap: 4 }}>
          {subcampProgress.map((subcamp) => (
            <Box key={subcamp.subcampId}>
              <Heading sx={{ fontSize: 1, mb: 2, color: 'fg.muted' }}>
                {subcamp.subcampName}
              </Heading>

              <Box
                borderWidth={1}
                borderStyle="solid"
                borderColor={subcamp.notStartedCount === 0 && subcamp.draftingCount === 0 ? 'success.emphasis' : 'border.default'}
                borderRadius={2}
                bg={subcamp.notStartedCount === 0 && subcamp.draftingCount === 0 ? 'success.subtle' : 'canvas.default'}
                overflow="hidden"
              >
              <Box
                p={3}
                display="flex"
                justifyContent="space-between"
                alignItems="center"
              >
                <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                  <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
                    {subcamp.submittedCount}/{subcamp.patrols.length}
                  </Text>
                  {subcamp.submittedCount === subcamp.patrols.length ? (
                    <Label variant="success">Complete ✓</Label>
                  ) : subcamp.completeCount > 0 ? (
                    <Label variant="accent">Complete</Label>
                  ) : subcamp.draftingCount > 0 || subcamp.notStartedCount > 0 ? (
                    <Label variant="attention">Incomplete</Label>
                  ) : (
                    <Label>Not Started</Label>
                  )}
                </Box>
                <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
                  {subcamp.submittedCount} finalised · {subcamp.completeCount} complete · {subcamp.draftingCount + subcamp.notStartedCount} incomplete
                </Text>
              </Box>
              <Box
                px={3}
                pb={3}
                display="flex"
                flexWrap="wrap"
                sx={{ gap: 1 }}
              >
                  {subcamp.patrols.map((patrol) => {
                    const displayState = displayStateForPatrol(patrol.overallStatus);
                    return (
                      <Box
                        key={patrol.patrolId}
                        title={`${patrol.patrolName}: ${displayState}`}
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          px: 2,
                          py: 1,
                          borderRadius: 2,
                          fontSize: 0,
                          fontWeight: 'bold',
                          bg: DISPLAY_STATE_COLORS[displayState],
                          color: 'fg.onEmphasis',
                          minWidth: '60px',
                          textAlign: 'center',
                        }}
                      >
                        <Text sx={{ fontSize: 0, mr: 1, color: 'inherit' }}>
                          {patrol.patrolName.length > 8
                            ? patrol.patrolName.slice(0, 8) + '...'
                            : patrol.patrolName}
                        </Text>
                        <Text sx={{ fontSize: 0, color: 'inherit' }}>
                          {DISPLAY_STATE_LABELS[displayState]}
                        </Text>
                      </Box>
                    );
                  })}
                </Box>
              </Box>
            </Box>
          ))}
        </Box>

        {subcampProgress.length === 0 && (
          <Box textAlign="center" py={6}>
            <Text sx={{ color: 'fg.muted', fontSize: 2 }}>
              No subcamp progress available for this session.
            </Text>
          </Box>
        )}
      </Box>
      )}
    </Box>
  );
};
