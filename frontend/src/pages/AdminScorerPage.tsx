import { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, Label, CounterLabel,
} from '@primer/react';
import * as api from '../lib/api';
import type { AdminPatrolScores } from '../lib/api';

export const AdminScorerPage = () => {
  const { sessionId, userId } = useParams<{ sessionId: string; userId: string }>();
  const navigate = useNavigate();

  const [displayName, setDisplayName] = useState('');
  const [sessionName, setSessionName] = useState('');
  const [criteria, setCriteria] = useState<{ id: string; title: string; description: string; min_value: number; max_value: number; sort_order: number }[]>([]);
  const [patrols, setPatrols] = useState<AdminPatrolScores[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  // Which patrol is expanded (null = none)
  const [expandedPatrol, setExpandedPatrol] = useState<string | null>(null);

  useEffect(() => {
    if (!sessionId || !userId) return;

    api.getAdminUserScores(sessionId, userId)
      .then((data) => {
        setDisplayName(data.display_name);
        setSessionName(data.session_name);
        setCriteria(data.criteria);
        setPatrols(data.patrols);
        // Auto-expand first patrol if there's only one, or expand all by default
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [sessionId, userId]);

  const togglePatrol = useCallback((patrolId: string) => {
    setExpandedPatrol((prev) => (prev === patrolId ? null : patrolId));
  }, []);

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
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
        <Box mb={2}>
          <Button
            variant="invisible"
            onClick={() => navigate(`/admin/sessions/${sessionId}`)}
            size="small"
          >
            ← Back to {sessionName}
          </Button>
        </Box>
        <Heading sx={{ fontSize: 3, mb: 1 }}>{displayName}</Heading>
        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
          {sessionName} — Scores &amp; Comments (Read-only)
        </Text>
      </Box>

      {error && (
        <Flash variant="danger" sx={{ m: 3 }}>
          {error}
        </Flash>
      )}

      {/* Patrol list */}
      <Box flex={1} p={3} overflow="auto">
        {patrols.length === 0 ? (
          <Box textAlign="center" py={6}>
            <Text sx={{ color: 'fg.muted', fontSize: 2 }}>
              This scorer has not submitted any scores yet.
            </Text>
          </Box>
        ) : (
          <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
            {patrols.map((patrol) => {
              const isOpen = expandedPatrol === patrol.patrol_id;
              const scoreMap: Record<string, number> = {};
              for (const s of patrol.scores) {
                scoreMap[s.criterion_id] = s.value;
              }
              const total = patrol.scores.reduce((sum, s) => sum + s.value, 0);
              const perUserComments = patrol.comments ?? [];
              const commentCount = perUserComments.length;

              return (
                <Box
                  key={patrol.patrol_id}
                  borderWidth={1}
                  borderStyle="solid"
                  borderColor="border.default"
                  borderRadius={2}
                  overflow="hidden"
                >
                  {/* Patrol header — clickable */}
                  <Box
                    as="button"
                    onClick={() => togglePatrol(patrol.patrol_id)}
                    sx={{
                      width: '100%',
                      p: 3,
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      bg: 'canvas.default',
                      border: 'none',
                      cursor: 'pointer',
                      ':hover': { bg: 'canvas.subtle' },
                    }}
                  >
                    <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                      <Text sx={{ fontWeight: 'bold', fontSize: 2 }}>
                        {patrol.patrol_name}
                      </Text>
                      <Label variant="success" size="small">
                        {total} pts
                      </Label>
                      {commentCount > 0 && (
                        <CounterLabel>💬 {commentCount}</CounterLabel>
                      )}
                    </Box>
                    <Text sx={{ fontSize: 1, color: 'fg.muted' }}>
                      {isOpen ? '▲' : '▼'}
                    </Text>
                  </Box>

                  {/* Expanded: show each criterion score + per-user comments */}
                  {isOpen && (
                    <Box
                      px={3}
                      pb={3}
                      borderTopWidth={1}
                      borderTopStyle="solid"
                      borderTopColor="border.default"
                    >
                      <Box display="flex" flexDirection="column" sx={{ gap: 3 }} pt={3}>
                        {criteria.map((criterion) => {
                          const value = scoreMap[criterion.id];
                          const criterionComments = perUserComments.filter(
                            (c) => c.criterion_id === criterion.id && c.comment.length > 0,
                          );
                          const range = criterion.max_value - criterion.min_value;
                          const pct = range > 0 && value !== undefined
                            ? ((value - criterion.min_value) / range) * 100
                            : 0;

                          return (
                            <Box key={criterion.id}>
                              {/* Criterion header */}
                              <Box display="flex" justifyContent="space-between" alignItems="baseline" mb={1}>
                                <Text sx={{ fontWeight: 'bold', fontSize: 2 }}>
                                  {criterion.title}
                                </Text>
                                <Text
                                  sx={{
                                    fontSize: 3,
                                    fontWeight: 'bold',
                                    fontVariantNumeric: 'tabular-nums',
                                    color: 'fg.muted',
                                  }}
                                >
                                  {value !== undefined ? value : '–'}
                                </Text>
                              </Box>

                              {criterion.description && (
                                <Text sx={{ color: 'fg.muted', fontSize: 0, mb: 2, display: 'block' }}>
                                  {criterion.description}
                                </Text>
                              )}

                              {/* Slider (read-only visual) */}
                              <Box position="relative">
                                <input
                                  type="range"
                                  min={criterion.min_value}
                                  max={criterion.max_value}
                                  step={1}
                                  value={value ?? criterion.min_value}
                                  disabled
                                  style={{
                                    width: '100%',
                                    height: '48px',
                                    cursor: 'not-allowed',
                                    accentColor: 'var(--fgColor-muted, #656d76)',
                                    opacity: 0.5,
                                  }}
                                />
                                <Box
                                  position="absolute"
                                  bottom={0}
                                  left={0}
                                  right={0}
                                  display="flex"
                                  justifyContent="space-between"
                                >
                                  <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{criterion.min_value}</Text>
                                  <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{criterion.max_value}</Text>
                                </Box>
                              </Box>

                              {/* Value bar */}
                              <Box mt={1} height="4px" borderRadius={2} bg="neutral.muted" overflow="hidden">
                                <Box
                                  height="100%"
                                  borderRadius={2}
                                  bg="neutral.emphasis"
                                  sx={{ width: `${pct}%`, transition: 'width 0.1s ease-out' }}
                                />
                              </Box>

                              {/* Per-user comments */}
                              {criterionComments.length > 0 && (
                                <Box mt={2} display="flex" flexDirection="column" sx={{ gap: 1 }}>
                                  {criterionComments.map((c) => (
                                    <Box key={`${c.user_id}-${c.criterion_id}`} p={2} bg="canvas.subtle" borderRadius={2}>
                                      <Text sx={{ fontSize: 0 }}>
                                        <Text sx={{ fontWeight: 'bold', color: 'fg.default' }}>{c.display_name}:</Text>{' '}
                                        <Text sx={{ color: 'fg.muted', fontStyle: 'italic' }}>{c.comment}</Text>
                                      </Text>
                                    </Box>
                                  ))}
                                </Box>
                              )}
                            </Box>
                          );
                        })}
                      </Box>
                    </Box>
                  )}
                </Box>
              );
            })}
          </Box>
        )}
      </Box>
    </Box>
  );
};
