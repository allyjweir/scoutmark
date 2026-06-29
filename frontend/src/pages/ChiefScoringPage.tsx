import { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, Label,
} from '@primer/react';
import type { Session, Criterion } from '../lib/types';
import * as api from '../lib/api';
import { useAuth } from '../hooks/useAuth';
import { ScoreSlider } from '../components/ScoreSlider';

type ChiefView = 'list' | 'detail' | 'completed';

export const ChiefScoringPage = () => {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();
  const { user } = useAuth();

  const [session, setSession] = useState<Session | null>(null);
  const [chiefRound, setChiefRound] = useState<api.ChiefRound | null>(null);
  const [message, setMessage] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [view, setView] = useState<ChiefView>('list');

  // Detail view state
  const [selectedPatrolId, setSelectedPatrolId] = useState<string | null>(null);
  const [originalScores, setOriginalScores] = useState<{ criterion_id: string; value: number; comment: string }[]>([]);
  const [chiefScores, setChiefScores] = useState<Record<string, number>>({});
  const [criteria, setCriteria] = useState<Criterion[]>([]);
  const [saving, setSaving] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [winnerPatrolId, setWinnerPatrolId] = useState<string | null>(null);

  // Load data
  useEffect(() => {
    if (!sessionId) return;
    const load = async () => {
      try {
        setLoading(true);
        const [sessionRes, chiefRes] = await Promise.all([
          api.getSession(sessionId),
          api.getChiefRound(sessionId),
        ]);
        setSession(sessionRes.session);
        setCriteria(sessionRes.session.criteria ?? []);
        setChiefRound(chiefRes.chief_round);
        setMessage(chiefRes.message ?? '');

        if (chiefRes.chief_round?.status === 'completed') {
          setView('completed');
          setWinnerPatrolId(chiefRes.chief_round.winner_patrol_id);
        }
      } catch (e: any) {
        setError(e.message ?? 'Failed to load');
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [sessionId]);

  const openPatrol = useCallback(async (patrolId: string) => {
    if (!sessionId || !chiefRound) return;
    try {
      setSelectedPatrolId(patrolId);
      const res = await api.getChiefPatrolOriginalScores(sessionId, patrolId);
      setOriginalScores(res.scores);

      // Load existing chief scores for this patrol
      const existingScores = chiefRound.scores?.[patrolId] ?? [];
      const scoreMap: Record<string, number> = {};
      for (const s of existingScores) {
        scoreMap[s.criterion_id] = s.value;
      }
      setChiefScores(scoreMap);
      setView('detail');
    } catch (e: any) {
      setError(e.message ?? 'Failed to load patrol scores');
    }
  }, [sessionId, chiefRound]);

  const handleScoreChange = (criterionId: string, value: number) => {
    setChiefScores(prev => ({ ...prev, [criterionId]: value }));
  };

  const handleSaveScores = useCallback(async () => {
    if (!sessionId || !selectedPatrolId) return;
    setSaving(true);
    try {
      await api.saveChiefScores(sessionId, selectedPatrolId, chiefScores);
      // Refresh chief round data
      const chiefRes = await api.getChiefRound(sessionId);
      setChiefRound(chiefRes.chief_round);
      setView('list');
      setSelectedPatrolId(null);
    } catch (e: any) {
      setError(e.message ?? 'Failed to save scores');
    } finally {
      setSaving(false);
    }
  }, [sessionId, selectedPatrolId, chiefScores]);

  const handleComplete = useCallback(async () => {
    if (!sessionId || !winnerPatrolId) return;
    setSubmitting(true);
    try {
      await api.completeChiefRound(sessionId, winnerPatrolId);
      const chiefRes = await api.getChiefRound(sessionId);
      setChiefRound(chiefRes.chief_round);
      setView('completed');
    } catch (e: any) {
      setError(e.message ?? 'Failed to complete chief round');
    } finally {
      setSubmitting(false);
    }
  }, [sessionId, winnerPatrolId]);

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" p={5}>
        <Spinner size="large" />
      </Box>
    );
  }

  if (error) {
    return <Flash variant="danger" sx={{ m: 3 }}>{error}</Flash>;
  }

  // Non camp-chief users see waiting message
  if (user?.role !== 'camp_chief') {
    return (
      <Box p={4}>
        <Heading sx={{ mb: 3 }}>{session?.name}</Heading>
        <Flash variant="warning">
          {message || "Awaiting camp chief's final review"}
        </Flash>
        <Button sx={{ mt: 3 }} onClick={() => navigate('/')}>Back to Dashboard</Button>
      </Box>
    );
  }

  // Chief round not ready
  if (!chiefRound) {
    return (
      <Box p={4}>
        <Heading sx={{ mb: 3 }}>{session?.name} — Chief Round</Heading>
        <Flash variant="warning">
          {message || 'Awaiting all scorers to submit'}
        </Flash>
        <Button sx={{ mt: 3 }} onClick={() => navigate('/')}>Back to Dashboard</Button>
      </Box>
    );
  }

  // Completed view
  if (view === 'completed') {
    const winnerPatrol = chiefRound.patrols?.find(p => p.patrol_id === chiefRound.winner_patrol_id);
    return (
      <Box p={4}>
        <Heading sx={{ mb: 3 }}>{session?.name} — Chief Round Complete</Heading>
        <Flash variant="success" sx={{ mb: 3 }}>
          🏆 Best Patrol: <strong>{winnerPatrol?.patrol_name ?? 'Unknown'}</strong>
          {winnerPatrol && <> (from {winnerPatrol.scorer_name})</>}
        </Flash>
        <Button onClick={() => navigate('/')}>Back to Dashboard</Button>
      </Box>
    );
  }

  // Detail view — scoring a specific patrol
  if (view === 'detail' && selectedPatrolId) {
    const patrol = chiefRound.patrols?.find(p => p.patrol_id === selectedPatrolId);
    const allScored = criteria.length > 0 && criteria.every(c => chiefScores[c.id] !== undefined);

    return (
      <Box p={4}>
        <Button variant="invisible" onClick={() => { setView('list'); setSelectedPatrolId(null); }} sx={{ mb: 3 }}>
          ← Back to patrols
        </Button>

        <Heading sx={{ mb: 2 }}>{patrol?.patrol_name}</Heading>
        <Text as="p" sx={{ mb: 3, color: 'fg.muted' }}>
          Scorer: {patrol?.scorer_name} · Original total: {patrol?.total_score}
        </Text>

        {/* Original scores & comments */}
        <Box mb={4}>
          <Heading as="h3" sx={{ fontSize: 2, mb: 2 }}>Original Scores & Comments</Heading>
          {criteria.map(criterion => {
            const orig = originalScores.find(s => s.criterion_id === criterion.id);
            return (
              <Box key={criterion.id} mb={2} p={2} borderWidth={1} borderStyle="solid" borderColor="border.default" borderRadius={2}>
                <Text sx={{ fontWeight: 'bold' }}>{criterion.title}</Text>
                <Text as="p" sx={{ fontSize: 1 }}>
                  Score: {orig?.value ?? '-'}/{criterion.max_value}
                  {orig?.comment && <> — <em>{orig.comment}</em></>}
                </Text>
              </Box>
            );
          })}
        </Box>

        {/* Chief scoring */}
        <Heading as="h3" sx={{ fontSize: 2, mb: 2 }}>Your Scores</Heading>
        {criteria.map(criterion => (
          <Box key={criterion.id} mb={3}>
            <ScoreSlider
              criterion={criterion}
              value={chiefScores[criterion.id] ?? null}
              comment=""
              onChange={(val) => handleScoreChange(criterion.id, val)}
              onCommentChange={() => {}}
            />
          </Box>
        ))}

        <Button
          variant="primary"
          disabled={!allScored || saving}
          onClick={handleSaveScores}
          sx={{ mt: 3 }}
        >
          {saving ? 'Saving...' : 'Save Scores'}
        </Button>
      </Box>
    );
  }

  // List view — show winning patrols
  const allPatrolsScored = chiefRound.patrols?.every(p =>
    chiefRound.scores?.[p.patrol_id]?.length === criteria.length
  ) ?? false;

  return (
    <Box p={4}>
      <Heading sx={{ mb: 2 }}>{session?.name} — Chief Round</Heading>
      <Text as="p" sx={{ mb: 3, color: 'fg.muted' }}>
        Score the winning patrol from each scorer, then declare the overall best patrol.
      </Text>

      {chiefRound.patrols?.map(patrol => {
        const scored = (chiefRound.scores?.[patrol.patrol_id]?.length ?? 0) === criteria.length;
        return (
          <Box
            key={patrol.patrol_id}
            p={3}
            mb={2}
            borderWidth={1}
            borderStyle="solid"
            borderColor={scored ? 'success.emphasis' : 'border.default'}
            borderRadius={2}
            sx={{ cursor: 'pointer', '&:hover': { bg: 'canvas.subtle' } }}
            onClick={() => openPatrol(patrol.patrol_id)}
          >
            <Box display="flex" justifyContent="space-between" alignItems="center">
              <Box>
                <Text sx={{ fontWeight: 'bold' }}>{patrol.patrol_name}</Text>
                <Text as="p" sx={{ fontSize: 1, color: 'fg.muted' }}>
                  From: {patrol.scorer_name} · Score: {patrol.total_score}
                </Text>
              </Box>
              {scored && <Label variant="success">Scored</Label>}
            </Box>
          </Box>
        );
      })}

      {/* Winner selection */}
      {allPatrolsScored && (
        <Box mt={4} p={3} borderWidth={1} borderStyle="solid" borderColor="accent.emphasis" borderRadius={2}>
          <Heading as="h3" sx={{ fontSize: 2, mb: 2 }}>Declare Overall Winner</Heading>
          <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
            {chiefRound.patrols?.map(patrol => (
              <Button
                key={patrol.patrol_id}
                variant={winnerPatrolId === patrol.patrol_id ? 'primary' : 'default'}
                onClick={() => setWinnerPatrolId(patrol.patrol_id)}
              >
                {patrol.patrol_name}
              </Button>
            ))}
          </Box>
          <Button
            variant="primary"
            disabled={!winnerPatrolId || submitting}
            onClick={handleComplete}
            sx={{ mt: 3 }}
            block
          >
            {submitting ? 'Submitting...' : '🏆 Declare Winner'}
          </Button>
        </Box>
      )}
    </Box>
  );
};
