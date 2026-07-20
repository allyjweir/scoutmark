import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Box, Button, CounterLabel, Flash, Heading, Label, Spinner, Text } from '@primer/react';
import * as api from '../lib/api';
import type { PatrolHistory } from '../lib/api';

const dateLabel = (date: string) => new Date(date).toLocaleDateString(undefined, {
  weekday: 'short', day: 'numeric', month: 'short', year: 'numeric',
});

export const PatrolDetailPage = () => {
  const { patrolId } = useParams<{ patrolId: string }>();
  const navigate = useNavigate();
  const [patrol, setPatrol] = useState<PatrolHistory | null>(null);
  const [expandedSession, setExpandedSession] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    api.getPatrolHistory()
      .then(({ patrols }) => setPatrol(patrols.find((item) => item.patrol_id === patrolId) ?? null))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [patrolId]);

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
      </Box>
    );
  }

  if (!patrol) {
    return (
      <Box p={3} maxWidth="600px" mx="auto">
        <Button variant="invisible" size="small" onClick={() => navigate('/patrols')} sx={{ mb: 3, px: 0 }}>
          ← Back to patrols
        </Button>
        <Flash variant="danger">{error || 'Patrol not found.'}</Flash>
      </Box>
    );
  }

  return (
    <Box p={3} maxWidth="600px" mx="auto">
      <Button variant="invisible" size="small" onClick={() => navigate('/patrols')} sx={{ mb: 3, px: 0 }}>
        ← All patrols
      </Button>
      <Heading sx={{ fontSize: 3, mb: 1 }}>{patrol.name}</Heading>
      <Text sx={{ color: 'fg.muted', display: 'block', mb: 3 }}>
        Score history. Tap a session to see the full breakdown and comments.
      </Text>

      {error && <Flash variant="danger" sx={{ mb: 3 }}>{error}</Flash>}

      {patrol.sessions.length === 0 ? (
        <Box textAlign="center" py={6}>
          <Text sx={{ color: 'fg.muted', fontSize: 2 }}>No scores have been submitted for this patrol yet.</Text>
        </Box>
      ) : (
        <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
          {patrol.sessions.map((session) => {
            const open = expandedSession === session.id;
            const commentCount = session.scores.reduce((total, score) => total + score.comments.length, 0);
            return (
              <Box key={session.id} borderWidth={1} borderStyle="solid" borderColor="border.default" borderRadius={2} overflow="hidden">
                <Box
                  as="button"
                  onClick={() => setExpandedSession(open ? null : session.id)}
                  sx={{
                    width: '100%',
                    p: 3,
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    textAlign: 'left',
                    bg: 'canvas.default',
                    border: 'none',
                    cursor: 'pointer',
                    ':hover': { bg: 'canvas.subtle' },
                  }}
                >
                  <Box>
                    <Text sx={{ display: 'block', fontWeight: 'bold' }}>{session.name}</Text>
                    <Text sx={{ color: 'fg.muted', fontSize: 0 }}>Submitted {dateLabel(session.submitted_at)}</Text>
                  </Box>
                  <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                    {commentCount > 0 && <CounterLabel>💬 {commentCount}</CounterLabel>}
                    <Label variant="success">{session.total} pts</Label>
                    <Text sx={{ color: 'fg.muted' }}>{open ? '▲' : '▼'}</Text>
                  </Box>
                </Box>

                {open && (
                  <Box px={3} pb={3} pt={1} borderTopWidth={1} borderTopStyle="solid" borderTopColor="border.default">
                    {session.scores.map((score) => (
                      <Box key={score.criterion_id} py={2} borderBottomWidth={1} borderBottomStyle="solid" borderBottomColor="border.muted">
                        <Box display="flex" justifyContent="space-between" sx={{ gap: 2 }}>
                          <Text sx={{ fontWeight: 'bold' }}>{score.criterion_title}</Text>
                          <Text sx={{ fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }}>
                            {score.value}/{score.max_value}
                          </Text>
                        </Box>
                        {score.comments.map((comment) => (
                          <Box key={comment.id} mt={2} pl={2} borderLeftWidth={2} borderLeftStyle="solid" borderLeftColor="accent.muted">
                            <Text sx={{ fontSize: 0, fontWeight: 'bold' }}>{comment.display_name}</Text>
                            <Text sx={{ display: 'block', fontSize: 1 }}>{comment.comment}</Text>
                          </Box>
                        ))}
                      </Box>
                    ))}
                  </Box>
                )}
              </Box>
            );
          })}
        </Box>
      )}
    </Box>
  );
};
