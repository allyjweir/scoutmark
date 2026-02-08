import { useState, useEffect, useCallback, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, ProgressBar,
  Label, CounterLabel,
} from '@primer/react';
import { keyBy, mapValues, every } from 'lodash';
import type { Session, Patrol, Submission } from '../lib/types';
import * as api from '../lib/api';
import { useDraftSync, useSessionSubscription } from '../hooks/useWebSocket';
import { ScoreSlider } from '../components/ScoreSlider';

type View = 'scoring' | 'summary' | 'viewing';

export const ScoringPage = () => {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();

  const [session, setSession] = useState<Session | null>(null);
  const [patrols, setPatrols] = useState<Patrol[]>([]);
  const [submissions, setSubmissions] = useState<Submission[]>([]);
  const [currentPatrolIndex, setCurrentPatrolIndex] = useState(0);
  const [scores, setScores] = useState<Record<string, number | null>>({});
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [view, setView] = useState<View>('scoring');
  const [jumpedFromSummary, setJumpedFromSummary] = useState(false);
  const [viewingScores, setViewingScores] = useState<Record<string, number>>({});
  const [revising, setRevising] = useState(false);

  // Track total score per patrol (patrol_id → total)
  const [patrolTotals, setPatrolTotals] = useState<Record<string, number | null>>({});

  // Track which patrols have had all criteria touched (keyed by patrol_id)
  const [touchedMap, setTouchedMap] = useState<Record<string, Set<string>>>({});

  const currentPatrol = patrols[currentPatrolIndex];
  const criteria = session?.criteria ?? [];

  // Draft sync over WebSocket
  const { saveDraft, flushDraft } = useDraftSync(
    sessionId ?? '',
    currentPatrol?.patrol_id ?? '',
  );

  // Subscribe to session updates
  useSessionSubscription(sessionId);

  // Load session data
  useEffect(() => {
    if (!sessionId) return;

    api.getSession(sessionId)
      .then(({ session, patrols, submissions }) => {
        setSession(session);
        setPatrols(patrols);
        setSubmissions(submissions);

        // If all patrols already submitted, go to summary
        const submittedIds = new Set(submissions.map((s) => s.patrol_id));
        const allDone = patrols.length > 0 && every(patrols, (p) => submittedIds.has(p.patrol_id));
        if (allDone) {
          setView('summary');
          // Load totals for submitted patrols
          for (const patrol of patrols) {
            api.getSubmissionScores(sessionId, patrol.patrol_id).then(({ scores }) => {
              const total = scores.reduce((sum, s) => sum + s.value, 0);
              setPatrolTotals((prev) => ({ ...prev, [patrol.patrol_id]: total }));
            }).catch(() => { /* ignore */ });
          }
        } else {
          // Start at first unsubmitted patrol
          const firstUnsubmitted = patrols.findIndex(
            (p) => !submittedIds.has(p.patrol_id),
          );
          if (firstUnsubmitted >= 0) {
            setCurrentPatrolIndex(firstUnsubmitted);
          }
        }
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [sessionId]);

  // Load draft when patrol changes
  useEffect(() => {
    if (!sessionId || !currentPatrol || view !== 'scoring') return;

    api.getDraft(sessionId, currentPatrol.patrol_id).then(({ draft }) => {
      if (draft?.scores?.length) {
        const restored: Record<string, number | null> = mapValues(
          keyBy(draft.scores, 'criterion_id'),
          'value',
        );
        setScores(restored);

        // Mark all restored criteria as touched
        setTouchedMap((prev) => ({
          ...prev,
          [currentPatrol.patrol_id]: new Set(draft.scores.map((s) => s.criterion_id)),
        }));

        // Update patrol total
        const total = draft.scores.reduce((sum, s) => sum + s.value, 0);
        setPatrolTotals((prev) => ({ ...prev, [currentPatrol.patrol_id]: total }));
      } else {
        // Initialize with null — slider shows at 0 but dimmed/unset
        const initial: Record<string, number | null> = {};
        for (const c of criteria) {
          initial[c.id] = null;
        }
        setScores(initial);
      }
    });
  }, [sessionId, currentPatrol?.patrol_id, criteria, view]);

  // Auto-save scores when they change (only save non-null values)
  const handleScoreChange = useCallback(
    (criterionId: string, value: number) => {
      setScores((prev) => {
        const next = { ...prev, [criterionId]: value };

        // Build only non-null scores for WebSocket save
        const saveable: Record<string, number> = {};
        let total = 0;
        for (const [k, v] of Object.entries(next)) {
          if (v !== null) {
            saveable[k] = v;
            total += v;
          }
        }
        if (Object.keys(saveable).length > 0) {
          saveDraft(saveable);
        }

        // Update patrol total
        if (currentPatrol) {
          setPatrolTotals((prev) => ({ ...prev, [currentPatrol.patrol_id]: total }));
        }

        return next;
      });

      // Track this criterion as touched for the current patrol
      if (currentPatrol) {
        setTouchedMap((prev) => {
          const existing = prev[currentPatrol.patrol_id] ?? new Set();
          const updated = new Set(existing);
          updated.add(criterionId);
          return { ...prev, [currentPatrol.patrol_id]: updated };
        });
      }
    },
    [saveDraft, currentPatrol],
  );

  // Check if all criteria for a patrol are touched
  const isPatrolComplete = useCallback(
    (patrolId: string) => {
      const touched = touchedMap[patrolId];
      if (!touched || criteria.length === 0) return false;
      return criteria.every((c) => touched.has(c.id));
    },
    [touchedMap, criteria],
  );

  // Navigate between patrols
  const goToPatrol = useCallback(
    (index: number) => {
      flushDraft();
      setCurrentPatrolIndex(index);
      setView('scoring');
      setJumpedFromSummary(false);
    },
    [flushDraft],
  );

  const goToSummary = useCallback(() => {
    flushDraft();
    setView('summary');
    setJumpedFromSummary(false);
  }, [flushDraft]);

  // Jump from summary to a specific patrol, with "Back to Summary" nav
  const jumpToPatrolFromSummary = useCallback(
    (index: number) => {
      flushDraft();
      setCurrentPatrolIndex(index);
      setView('scoring');
      setJumpedFromSummary(true);
    },
    [flushDraft],
  );

  const goNext = useCallback(() => {
    if (currentPatrolIndex < patrols.length - 1) {
      goToPatrol(currentPatrolIndex + 1);
    } else {
      // Last patrol — go to summary
      goToSummary();
    }
  }, [currentPatrolIndex, patrols.length, goToPatrol, goToSummary]);

  const goPrev = useCallback(() => {
    if (currentPatrolIndex > 0) {
      goToPatrol(currentPatrolIndex - 1);
    }
  }, [currentPatrolIndex, goToPatrol]);

  // Finalise — submit all patrols at once
  const handleFinalise = useCallback(async () => {
    if (!sessionId) return;
    setShowConfirmFinalise(false);
    setSubmitting(true);
    setError('');

    try {
      await flushDraft();
      const result = await api.finaliseSession(sessionId);
      setSubmissions(result.submissions);

      // Navigate to dashboard with success feedback
      navigate('/', { state: { finalised: session?.name ?? 'Session' } });
    } catch (err) {
      console.error('[finalise] Error:', err);
      setError(err instanceof Error ? err.message : 'Finalise failed');
      setSubmitting(false);
    }
  }, [sessionId, session, flushDraft, navigate]);

  // View submitted scores for a patrol (read-only)
  const viewPatrolScores = useCallback(
    async (patrolIndex: number) => {
      if (!sessionId) return;
      const patrol = patrols[patrolIndex];
      if (!patrol) return;

      try {
        const { scores: submissionScores } = await api.getSubmissionScores(
          sessionId,
          patrol.patrol_id,
        );
        const scoreMap: Record<string, number> = {};
        for (const s of submissionScores) {
          scoreMap[s.criterion_id] = s.value;
        }
        setViewingScores(scoreMap);
        setCurrentPatrolIndex(patrolIndex);
        setView('viewing');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Could not load scores');
      }
    },
    [sessionId, patrols],
  );

  // Revise — convert submissions back to drafts for editing
  const handleRevise = useCallback(async () => {
    if (!sessionId) return;
    setRevising(true);
    setError('');

    try {
      await api.reviseSession(sessionId);
      // Clear submissions and put user back in scoring mode
      setSubmissions([]);
      setTouchedMap({});
      setPatrolTotals({});
      setCurrentPatrolIndex(0);
      setView('scoring');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not revise scores');
    } finally {
      setRevising(false);
    }
  }, [sessionId]);

  // Progress tracking
  const submittedPatrolIds = useMemo(
    () => new Set(submissions.map((s) => s.patrol_id)),
    [submissions],
  );

  const isCurrentSubmitted = currentPatrol
    ? submittedPatrolIds.has(currentPatrol.patrol_id)
    : false;

  const allSubmitted = patrols.length > 0 &&
    every(patrols, (p) => submittedPatrolIds.has(p.patrol_id));

  // Incomplete-scores confirmation
  const [showConfirmFinalise, setShowConfirmFinalise] = useState(false);

  const incompletePatrols = useMemo(() => {
    if (!patrols.length || !criteria.length) return [];
    return patrols.filter((p) => {
      if (submittedPatrolIds.has(p.patrol_id)) return false;
      return !isPatrolComplete(p.patrol_id);
    });
  }, [patrols, criteria, submittedPatrolIds, isPatrolComplete]);

  const requestFinalise = useCallback(() => {
    if (incompletePatrols.length > 0) {
      setShowConfirmFinalise(true);
    } else {
      handleFinalise();
    }
  }, [incompletePatrols, handleFinalise]);

  const isLastPatrol = currentPatrolIndex === patrols.length - 1;

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
            ← Back
          </Button>
          <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
            {session.event_name}
          </Text>
        </Box>
        <Heading sx={{ fontSize: 2, mb: 1 }}>{session.name}</Heading>

        {/* Progress bar — tracks patrols that are scored or submitted */}
        {(() => {
          const readyCount = patrols.filter(
            (p) => submittedPatrolIds.has(p.patrol_id) || isPatrolComplete(p.patrol_id),
          ).length;
          return (
            <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
              <ProgressBar
                progress={(readyCount / patrols.length) * 100}
                sx={{ flex: 1 }}
              />
              <CounterLabel>
                {readyCount}/{patrols.length}
              </CounterLabel>
            </Box>
          );
        })()}
      </Box>

      {error && (
        <Flash variant="danger" sx={{ m: 3 }}>
          {error}
        </Flash>
      )}

      {/* ─── Summary view ─── */}
      {view === 'summary' && (
        <>
          <Box flex={1} p={3} overflow="auto">
            <Heading sx={{ fontSize: 3, mb: 3 }}>Review Scores</Heading>

            <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
              {patrols.map((patrol, index) => {
                const isSubmitted = submittedPatrolIds.has(patrol.patrol_id);
                const isComplete = isPatrolComplete(patrol.patrol_id);

                return (
                  <Box
                    key={patrol.patrol_id}
                    as="button"
                    onClick={
                      isSubmitted
                        ? () => viewPatrolScores(index)
                        : () => jumpToPatrolFromSummary(index)
                    }
                    sx={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      p: 3,
                      borderWidth: 1,
                      borderStyle: 'solid',
                      borderColor: 'border.default',
                      borderRadius: 2,
                      bg: 'canvas.default',
                      cursor: 'pointer',
                      textAlign: 'left',
                      width: '100%',
                      ':hover': { bg: 'canvas.subtle' },
                    }}
                  >
                    <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                      <Text sx={{ fontWeight: 'bold', fontSize: 2 }}>
                        {patrol.name}
                      </Text>
                      {isSubmitted && (
                        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>Tap to view</Text>
                      )}
                    </Box>
                    <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                      {patrolTotals[patrol.patrol_id] != null && (
                        <Text sx={{ fontSize: 1, color: 'fg.muted', whiteSpace: 'nowrap' }}>
                          Total: {patrolTotals[patrol.patrol_id]}/{criteria.reduce((sum, c) => sum + c.max_value, 0)}
                        </Text>
                      )}
                      {isSubmitted ? (
                        <Label variant="success">Submitted ✓</Label>
                      ) : isComplete ? (
                        <Label variant="accent">Scores set ✓</Label>
                      ) : (
                        <Label variant="attention">Incomplete</Label>
                      )}
                    </Box>
                  </Box>
                );
              })}
            </Box>
          </Box>

          {/* Finalise bar */}
          <Box
            p={3}
            borderTopWidth={1}
            borderTopStyle="solid"
            borderTopColor="border.default"
            bg="canvas.subtle"
            display="flex"
            sx={{ gap: 2 }}
          >
            {allSubmitted ? (
              <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Box textAlign="center" p={2}>
                  <Text sx={{ color: 'success.fg', fontWeight: 'bold', fontSize: 2 }}>
                    🎉 All patrols scored! Great work.
                  </Text>
                </Box>
                {session.status === 'ACTIVE' && (
                  <Button
                    onClick={handleRevise}
                    disabled={revising}
                    sx={{ width: '100%' }}
                    size="large"
                  >
                    {revising ? 'Reopening…' : '✏️ Revise Scores'}
                  </Button>
                )}
              </Box>
            ) : session.status === 'ACTIVE' ? (
              <>
                <Button
                  onClick={() => {
                    setView('scoring');
                    setCurrentPatrolIndex(patrols.length - 1);
                  }}
                  sx={{ flex: 1 }}
                  size="large"
                >
                  ← Prev
                </Button>
                <Button
                  variant="primary"
                  onClick={requestFinalise}
                  sx={{ flex: 2 }}
                  size="large"
                  disabled={submitting}
                >
                  {submitting ? 'Submitting…' : 'Finalise Scores'}
                </Button>
              </>
            ) : (
              <Box textAlign="center" p={2} sx={{ flex: 1 }}>
                <Text sx={{ color: 'fg.muted', fontSize: 1 }}>
                  This session is closed.
                </Text>
              </Box>
            )}
          </Box>

          {/* Incomplete scores confirmation overlay */}
          {showConfirmFinalise && (
            <Box
              position="fixed"
              top={0}
              left={0}
              right={0}
              bottom={0}
              bg="neutral.muted"
              display="flex"
              alignItems="center"
              justifyContent="center"
              sx={{ zIndex: 100 }}
              onClick={() => setShowConfirmFinalise(false)}
            >
              <Box
                bg="canvas.default"
                borderRadius={2}
                borderWidth={1}
                borderStyle="solid"
                borderColor="attention.emphasis"
                p={4}
                mx={3}
                maxWidth="400px"
                sx={{ width: '100%' }}
                onClick={(e: React.MouseEvent) => e.stopPropagation()}
              >
                <Heading sx={{ fontSize: 2, mb: 2 }}>⚠️ Incomplete Scores</Heading>
                <Text as="p" sx={{ fontSize: 1, mb: 2, color: 'fg.muted' }}>
                  The following patrols have unset criteria that will be submitted as <strong>zero</strong>:
                </Text>
                <Box as="ul" sx={{ pl: 3, mb: 3 }}>
                  {incompletePatrols.map((p) => (
                    <Box as="li" key={p.patrol_id} sx={{ fontSize: 1, mb: 1 }}>
                      <Text sx={{ fontWeight: 'bold' }}>{p.name}</Text>
                      <Text sx={{ color: 'fg.muted' }}>
                        {' '}— {(touchedMap[p.patrol_id]?.size ?? 0)}/{criteria.length} criteria set
                      </Text>
                    </Box>
                  ))}
                </Box>
                <Box display="flex" sx={{ gap: 2 }}>
                  <Button
                    onClick={() => setShowConfirmFinalise(false)}
                    sx={{ flex: 1 }}
                    size="large"
                  >
                    Go Back
                  </Button>
                  <Button
                    variant="danger"
                    onClick={handleFinalise}
                    sx={{ flex: 1 }}
                    size="large"
                    disabled={submitting}
                  >
                    {submitting ? 'Submitting…' : 'Submit Anyway'}
                  </Button>
                </Box>
              </Box>
            </Box>
          )}
        </>
      )}

      {/* ─── Scoring view ─── */}
      {view === 'scoring' && (
        <>
          {/* Patrol selector strip */}
          <Box
            display="flex"
            overflowX="auto"
            p={2}
            sx={{ gap: 1 }}
            borderBottomWidth={1}
            borderBottomStyle="solid"
            borderBottomColor="border.default"
          >
            {patrols.map((patrol, index) => {
              const isSubmitted = submittedPatrolIds.has(patrol.patrol_id);
              const isComplete = isPatrolComplete(patrol.patrol_id);
              const isCurrent = index === currentPatrolIndex;

              return (
                <Button
                  key={patrol.patrol_id}
                  variant={isCurrent ? 'primary' : 'invisible'}
                  size="small"
                  onClick={() => goToPatrol(index)}
                  sx={{
                    flexShrink: 0,
                    position: 'relative',
                  }}
                >
                  {patrol.name}
                  {(isSubmitted || isComplete) && (
                    <Label
                      variant={isSubmitted ? 'success' : 'accent'}
                      sx={{ ml: 1 }}
                    >
                      ✓
                    </Label>
                  )}
                </Button>
              );
            })}
          </Box>

          {/* Scoring area */}
          {currentPatrol && (
            <Box flex={1} p={3} overflow="auto">
              <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
                <Heading sx={{ fontSize: 3 }}>{currentPatrol.name}</Heading>
                {isCurrentSubmitted && (
                  <Label variant="success" size="large">Submitted ✓</Label>
                )}
              </Box>

              {/* Criteria sliders */}
              <Box display="flex" flexDirection="column" sx={{ gap: 4 }}>
                {criteria.map((criterion) => (
                  <ScoreSlider
                    key={criterion.id}
                    criterion={criterion}
                    value={scores[criterion.id] ?? null}
                    onChange={(value) => handleScoreChange(criterion.id, value)}
                    disabled={isCurrentSubmitted || session.status !== 'ACTIVE'}
                  />
                ))}
              </Box>
            </Box>
          )}

          {/* Bottom navigation bar */}
          <Box
            p={3}
            borderTopWidth={1}
            borderTopStyle="solid"
            borderTopColor="border.default"
            bg="canvas.subtle"
            display="flex"
            sx={{ gap: 2 }}
          >
            {jumpedFromSummary ? (
              /* Jumped from summary — single "Back to Summary" button */
              <Button
                variant="primary"
                onClick={goToSummary}
                sx={{ flex: 1 }}
                size="large"
              >
                ← Back to Summary
              </Button>
            ) : (
              /* Normal Prev / Next flow */
              <>
                <Button
                  onClick={goPrev}
                  disabled={currentPatrolIndex === 0}
                  sx={{ flex: 1 }}
                  size="large"
                >
                  ← Prev
                </Button>

                <Button
                  onClick={goNext}
                  sx={{ flex: 1 }}
                  size="large"
                >
                  {isLastPatrol ? 'Review →' : 'Next →'}
                </Button>
              </>
            )}
          </Box>
        </>
      )}

      {/* ─── Viewing submitted scores (read-only) ─── */}
      {view === 'viewing' && currentPatrol && (
        <>
          <Box flex={1} p={3} overflow="auto">
            <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
              <Heading sx={{ fontSize: 3 }}>{currentPatrol.name}</Heading>
              <Label variant="success" size="large">Submitted ✓</Label>
            </Box>

            <Box display="flex" flexDirection="column" sx={{ gap: 4 }}>
              {criteria.map((criterion) => (
                <ScoreSlider
                  key={criterion.id}
                  criterion={criterion}
                  value={viewingScores[criterion.id] ?? null}
                  onChange={() => {}}
                  disabled
                />
              ))}
            </Box>
          </Box>

          <Box
            p={3}
            borderTopWidth={1}
            borderTopStyle="solid"
            borderTopColor="border.default"
            bg="canvas.subtle"
          >
            <Button
              variant="primary"
              onClick={() => setView('summary')}
              sx={{ width: '100%' }}
              size="large"
            >
              ← Back to Summary
            </Button>
          </Box>
        </>
      )}

    </Box>
  );
};
