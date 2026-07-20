import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Box, Button, Flash, Heading, Spinner, Text } from '@primer/react';
import type { Patrol, Session, Submission } from '../lib/types';
import * as api from '../lib/api';
import { ScoreSlider } from '../components/ScoreSlider';

export const AdminPatrolScoresPage = () => {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();
  const [session, setSession] = useState<Session | null>(null);
  const [patrols, setPatrols] = useState<Patrol[]>([]);
  const [submissions, setSubmissions] = useState<Submission[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [scores, setScores] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  useEffect(() => {
    if (!sessionId) return;
    api.getSession(sessionId).then(({ session: loaded, patrols: loadedPatrols, submissions: loadedSubmissions }) => {
      setSession(loaded);
      setPatrols(loadedPatrols);
      setSubmissions(loadedSubmissions);
    }).catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, [sessionId]);

  const submitted = useMemo(() => new Set(submissions.map((submission) => submission.patrol_id)), [submissions]);
  const selectedPatrol = patrols.find((patrol) => patrol.patrol_id === selected);

  const selectPatrol = async (patrolId: string) => {
    if (!sessionId) return;
    setSelected(patrolId);
    setConfirming(false);
    setError('');
    try {
      const { scores: saved } = await api.getSubmissionScores(sessionId, patrolId);
      setScores(Object.fromEntries(saved.map((score) => [score.criterion_id, score.value])));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not load scores');
    }
  };

  const save = async () => {
    if (!sessionId || !selected) return;
    setSaving(true);
    setError('');
    try {
      await api.updateAdminPatrolScores(sessionId, selected, scores);
      setNotice(`Scores for ${selectedPatrol?.name} updated.`);
      setConfirming(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not update scores');
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh"><Spinner size="large" /></Box>;
  if (!session) return <Box p={3}><Flash variant="danger">Session not found.</Flash></Box>;

  return <Box p={3} maxWidth="720px" mx="auto">
    <Button variant="invisible" size="small" onClick={() => navigate('/admin')}>← Admin dashboard</Button>
    <Heading sx={{ fontSize: 3, mt: 2 }}>{session.name}</Heading>
    <Text sx={{ color: 'fg.muted', fontSize: 1, display: 'block', mb: 3 }}>Manual patrol score corrections</Text>
    {error && <Flash variant="danger" sx={{ mb: 3 }}>{error}</Flash>}
    {notice && <Flash variant="success" sx={{ mb: 3 }}>{notice}</Flash>}
    {!selected ? <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
      {patrols.map((patrol) => <Box key={patrol.patrol_id} p={3} borderWidth={1} borderStyle="solid" borderColor="border.default" borderRadius={2} display="flex" justifyContent="space-between" alignItems="center">
        <Box><Text sx={{ fontWeight: 'bold', display: 'block' }}>{patrol.subcamp ? `${patrol.subcamp} — ` : ''}{patrol.name}</Text><Text sx={{ fontSize: 0, color: 'fg.muted' }}>{submitted.has(patrol.patrol_id) ? 'Submitted scores' : 'No submitted scores'}</Text></Box>
        <Button size="small" disabled={!submitted.has(patrol.patrol_id)} onClick={() => selectPatrol(patrol.patrol_id)}>Edit</Button>
      </Box>)}
    </Box> : <Box>
      <Button size="small" onClick={() => setSelected(null)} sx={{ mb: 3 }}>Choose another patrol</Button>
      <Heading sx={{ fontSize: 2, mb: 2 }}>{selectedPatrol?.name}</Heading>
      <Flash variant="warning" sx={{ mb: 3 }}>This directly changes submitted scores, including closed or locked historical sessions. Review every value before confirming.</Flash>
      <Box display="flex" flexDirection="column" sx={{ gap: 3 }}>
        {(session.criteria ?? []).map((criterion) => <ScoreSlider key={criterion.id} criterion={criterion} value={scores[criterion.id] ?? null} comment="" otherComments={[]} onChange={(value) => setScores((previous) => ({ ...previous, [criterion.id]: value }))} onCommentChange={() => {}} />)}
      </Box>
      {!confirming ? <Button variant="danger" sx={{ mt: 4 }} onClick={() => setConfirming(true)}>Review and save correction</Button> : <Box mt={4} p={3} borderWidth={1} borderStyle="solid" borderColor="danger.emphasis" borderRadius={2}>
        <Text sx={{ fontWeight: 'bold', display: 'block', mb: 2 }}>Confirm score correction?</Text>
        <Text sx={{ fontSize: 1, display: 'block', mb: 2 }}>This overwrites the historical submitted values and is effective immediately.</Text>
        <Box display="flex" sx={{ gap: 2 }}><Button variant="danger" disabled={saving} onClick={save}>{saving ? 'Saving…' : 'Confirm and save'}</Button><Button disabled={saving} onClick={() => setConfirming(false)}>Cancel</Button></Box>
      </Box>}
    </Box>}
  </Box>;
};
