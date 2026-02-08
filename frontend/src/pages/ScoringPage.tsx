import { useState, useEffect, useCallback, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, ProgressBar,
  Label, CounterLabel, Dialog,
} from '@primer/react';
import { keyBy, mapValues, some, every } from 'lodash';
import type { Session, Patrol, Criterion, Submission } from '../lib/types';
import * as api from '../lib/api';
import { useDraftSync, useSessionSubscription } from '../hooks/useWebSocket';
import { ScoreSlider } from '../components/ScoreSlider';

export const ScoringPage = () => {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();

  const [session, setSession] = useState<Session | null>(null);
  const [patrols, setPatrols] = useState<Patrol[]>([]);
  const [submissions, setSubmissions] = useState<Submission[]>([]);
  const [currentPatrolIndex, setCurrentPatrolIndex] = useState(0);
  const [scores, setScores] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [showConfirm, setShowConfirm] = useState(false);

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

        // Find first unsubmitted patrol
        const submittedPatrolIds = new Set(submissions.map((s) => s.patrol_id));
        const firstUnsubmitted = patrols.findIndex(
          (p) => !submittedPatrolIds.has(p.patrol_id),
        );
        if (firstUnsubmitted >= 0) {
          setCurrentPatrolIndex(firstUnsubmitted);
        }
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [sessionId]);

  // Load draft when patrol changes
  useEffect(() => {
    if (!sessionId || !currentPatrol) return;

    api.getDraft(sessionId, currentPatrol.patrol_id).then(({ draft }) => {
      if (draft?.scores?.length) {
        const restored = mapValues(
          keyBy(draft.scores, 'criterion_id'),
          'value',
        );
        setScores(restored);
      } else {
        // Initialize with midpoint values
        const initial = mapValues(
          keyBy(criteria, 'id'),
          (c: Criterion) => Math.floor((c.min_value + c.max_value) / 2),
        );
        setScores(initial);
      }
    });
  }, [sessionId, currentPatrol?.patrol_id, criteria]);

  // Auto-save scores when they change
  const handleScoreChange = useCallback(
    (criterionId: string, value: number) => {
      setScores((prev) => {
        const next = { ...prev, [criterionId]: value };
        saveDraft(next);
        return next;
      });
    },
    [saveDraft],
  );

  // Navigate between patrols
  const goToPatrol = useCallback(
    (index: number) => {
      flushDraft();
      setCurrentPatrolIndex(index);
    },
    [flushDraft],
  );

  const goNext = useCallback(() => {
    if (currentPatrolIndex < patrols.length - 1) {
      goToPatrol(currentPatrolIndex + 1);
    }
  }, [currentPatrolIndex, patrols.length, goToPatrol]);

  const goPrev = useCallback(() => {
    if (currentPatrolIndex > 0) {
      goToPatrol(currentPatrolIndex - 1);
    }
  }, [currentPatrolIndex, goToPatrol]);

  // Submit scores
  const handleSubmit = useCallback(async () => {
    if (!sessionId || !currentPatrol) return;
    setSubmitting(true);
    setError('');

    try {
      flushDraft();
      const submission = await api.submitScores(
        sessionId,
        currentPatrol.patrol_id,
        scores,
      );
      setSubmissions((prev) => [...prev, submission]);
      setShowConfirm(false);

      // Auto-advance to next unsubmitted patrol
      const allSubmittedIds = new Set([
        ...submissions.map((s) => s.patrol_id),
        currentPatrol.patrol_id,
      ]);
      const nextUnsubmitted = patrols.findIndex(
        (p, i) => i > currentPatrolIndex && !allSubmittedIds.has(p.patrol_id),
      );
      if (nextUnsubmitted >= 0) {
        goToPatrol(nextUnsubmitted);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Submission failed');
    } finally {
      setSubmitting(false);
    }
  }, [sessionId, currentPatrol, scores, flushDraft, submissions, patrols, currentPatrolIndex, goToPatrol]);

  // Progress tracking
  const submittedPatrolIds = useMemo(
    () => new Set(submissions.map((s) => s.patrol_id)),
    [submissions],
  );

  const isCurrentSubmitted = currentPatrol
    ? submittedPatrolIds.has(currentPatrol.patrol_id)
    : false;

  const allComplete = patrols.length > 0 &&
    every(patrols, (p) => submittedPatrolIds.has(p.patrol_id));

  const hasAllScores = criteria.length > 0 &&
    every(criteria, (c) => scores[c.id] !== undefined);

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

        {/* Progress bar */}
        <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
          <ProgressBar
            progress={(submittedPatrolIds.size / patrols.length) * 100}
            sx={{ flex: 1 }}
          />
          <CounterLabel>
            {submittedPatrolIds.size}/{patrols.length}
          </CounterLabel>
        </Box>
      </Box>

      {error && (
        <Flash variant="danger" sx={{ m: 3 }}>
          {error}
        </Flash>
      )}

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
              {isSubmitted && (
                <Label variant="success" sx={{ ml: 1 }}>✓</Label>
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
                value={scores[criterion.id] ?? criterion.min_value}
                onChange={(value) => handleScoreChange(criterion.id, value)}
                disabled={isCurrentSubmitted}
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
        <Button
          onClick={goPrev}
          disabled={currentPatrolIndex === 0}
          sx={{ flex: 1 }}
          size="large"
        >
          ← Prev
        </Button>

        {!isCurrentSubmitted ? (
          <Button
            variant="primary"
            onClick={() => setShowConfirm(true)}
            disabled={!hasAllScores}
            sx={{ flex: 2 }}
            size="large"
          >
            Submit Scores
          </Button>
        ) : (
          <Button
            variant="default"
            disabled
            sx={{ flex: 2 }}
            size="large"
          >
            Submitted ✓
          </Button>
        )}

        <Button
          onClick={goNext}
          disabled={currentPatrolIndex === patrols.length - 1}
          sx={{ flex: 1 }}
          size="large"
        >
          Next →
        </Button>
      </Box>

      {/* All complete banner */}
      {allComplete && (
        <Box p={3} bg="success.subtle" textAlign="center">
          <Text sx={{ color: 'success.fg', fontWeight: 'bold' }}>
            🎉 All patrols scored! Great work.
          </Text>
        </Box>
      )}

      {/* Confirmation dialog */}
      {showConfirm && (
        <Dialog
          title="Submit scores?"
          onClose={() => setShowConfirm(false)}
          footerButtons={[
            {
              content: 'Cancel',
              onClick: () => setShowConfirm(false),
            },
            {
              content: submitting ? 'Submitting…' : 'Submit',
              buttonType: 'primary',
              onClick: handleSubmit,
              disabled: submitting,
            },
          ]}
        >
          <Text>
            Submit scores for <strong>{currentPatrol?.name}</strong>?
            Once submitted, scores will be locked.
          </Text>
        </Dialog>
      )}
    </Box>
  );
};
